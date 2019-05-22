package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"

	sql "github.com/jmoiron/sqlx"

	_ "github.com/mattn/go-sqlite3"
)

type note struct {
	Title             string `db:"ZTITLE"`
	BodyRaw           []byte `db:"ZTEXT"`
	Body              string
	Date              string
	Hashtags          []string
	CustomFrontMatter []string
	Categories        bool
	Tags              bool
	Draft             bool
}

const templateRaw = `---
title: "{{ .Title }}"
date: {{ .Date }}

{{- if .Categories }}
categories: [
{{- range $i, $c := .Hashtags -}}
	{{- if $i -}},{{- end -}}
	"{{- $c -}}"
{{- end -}}
]
{{- end }}

{{- if .Tags }}
tags: [
{{- range $i, $c := .Hashtags -}}
	{{- if $i -}},{{- end -}}
	"{{- $c -}}"
{{- end -}}
]
{{- end }}
draft: {{ .Draft }}
{{- range $l := .CustomFrontMatter }}
{{ $l }}
{{- end }}
---
{{ .Body }}`

// Front matter that Bhguo manages.
var bhugoFrontMatter = map[string]bool{
	"title":      true,
	"date":       true,
	"categories": true,
	"tags":       true,
	"draft":      true,
}

func main() {
	log.Info("Bhugo Initializing")

	err := godotenv.Load(".bhugo")
	if err != nil {
		log.Fatal(err)
	}

	var cfg struct {
		Interval   time.Duration `default:"1s"`
		HugoDir    string        `split_words:"true" required:"true"`
		ContentDir string        `split_words:"true" default:"content/blog"`
		ImageDir   string        `split_words:"true" default:"/img/posts"`
		NoteTag    string        `split_words:"true" default:"blog"`
		Database   string        `required:"true"`
		Categories bool          `default:"true"`
		Tags       bool          `default:"false"`
	}

	err = envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Override these defaults with the configuration values.
	bhugoFrontMatter["categories"] = cfg.Categories
	bhugoFrontMatter["tags"] = cfg.Tags

	timeFormat := "2006-01-02T15:04:05-07:00"

	db, err := sql.Connect("sqlite3", cfg.Database)
	if err != nil {
		log.Fatal(err)
	}

	tmpl, err := template.New("Note Template").Parse(templateRaw)
	if err != nil {
		log.Fatal(err)
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 2)
	notes := make(chan note, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	wg := sync.WaitGroup{}

	log.Infof("Watching Bear tag #%s for changes", cfg.NoteTag)

	wg.Add(1)
	go checkBear(&wg, done, db, cfg.Interval, notes, cfg.NoteTag)

	wg.Add(1)
	go updateHugo(&wg, done, notes, time.Now, timeFormat, cfg.NoteTag, cfg.HugoDir, cfg.ContentDir, cfg.ImageDir, tmpl, cfg.Categories, cfg.Tags)

	go func() {
		sig := <-sigs
		log.Info(sig)
		done <- true
		done <- true
	}()

	wg.Wait()
	log.Info("Bhugo Exiting")
}

func checkBear(wg *sync.WaitGroup, done <-chan bool, db *sql.DB, interval time.Duration, notesChan chan<- note, noteTag string) {
	log.Debug("Starting CheckBear")

	defer wg.Done()

	tick := time.Tick(interval)
	cache := make(map[string][]byte)

	for {
		select {
		case <-tick:
			notes := []note{}
			q := fmt.Sprintf("SELECT ZTITLE, ZTEXT FROM ZSFNOTE WHERE ZTEXT LIKE '%%#%s%%'", noteTag)
			if err := db.Select(&notes, q); err != nil {
				log.Error(err)
				continue
			}

			// Initialize cache for any new notes with changes.
			for _, n := range notes {
				c, ok := cache[n.Title]
				if !ok {
					cache[n.Title] = n.BodyRaw
					continue
				}

				if !bytes.Equal(c, n.BodyRaw) {
					log.Infof("Differences detected in %s - updating Hugo", n.Title)
					cache[n.Title] = n.BodyRaw
					notesChan <- n
				}
			}

		case <-done:
			log.Info("Check Bear exiting")
			return
		}
	}
}

func updateHugo(wg *sync.WaitGroup, done <-chan bool, notes <-chan note, timeProvider func() time.Time, timeFormat, noteTag, hugoDir, contentDir, imageDir string, tmpl *template.Template, categories, tags bool) {
	log.Debug("Starting UpdateHugo")
	defer wg.Done()

	for {
		select {
		case n := <-notes:
			// Replace smart quotes with regular quotes.
			n.BodyRaw = bytes.Replace(n.BodyRaw, []byte("“"), []byte("\""), -1)
			n.BodyRaw = bytes.Replace(n.BodyRaw, []byte("”"), []byte("\""), -1)

			n.Date = timeProvider().Format(timeFormat)

			lines := bytes.Split(n.BodyRaw, []byte("\n"))
			// If there is only a heading and tags continue on.
			if len(lines) < 3 {
				continue
			}

			// The second line should be the line with tags.
			n.Hashtags = scanTags(lines[1], noteTag)
			for _, c := range n.Hashtags {
				if strings.Contains(strings.ToLower(c), "draft") {
					n.Draft = true
				}
			}

			// The Bear hashtags will populate either categories or tags (or both) depending on these bools.
			n.Categories = categories
			n.Tags = tags

			// Format images for Hugo.
			parseImages(lines, imageDir)

			// First two lines are the title of the note and the tags.
			n.Body = string(bytes.Join(lines[2:], []byte("\n")))
			target := strings.Replace(strings.ToLower(n.Title), " ", "-", -1)

			fp := fmt.Sprintf("%s/%s/%s.md", hugoDir, contentDir, target)

			cf, err := ioutil.ReadFile(fp)
			if err != nil && !os.IsNotExist(err) {
				log.Error(err)
				continue
			}
			// If the file exists, check for any custom front matter to preserve it.
			if len(cf) > 0 {
				n.CustomFrontMatter = customFrontMatter(cf)
			}

			f, err := os.Create(fp)
			if err != nil {
				log.Error(err)
				continue
			}

			if err := tmpl.Execute(f, n); err != nil {
				log.Error(err)
			}

			if err := f.Close(); err != nil {
				log.Error(err)
			}
		case <-done:
			log.Info("Update Hugo exiting")
			return
		}
	}
}

func scanTags(l []byte, tag string) []string {
	start := 0
	end := 0
	inHash := false
	multiWord := false
	hashtags := []string{}
	var prev rune

	for i, r := range l {
		var peek rune
		if i < (len(l) - 1) {
			peek = rune(l[i+1])
		} else {
			peek = 0
		}

		switch {
		// When a starting hashtag is found, initialize the starting point index.
		case r == '#' && (prev == ' ' || prev == 0) && !inHash:
			start = i + 1
			inHash = true
			end = start

		// When the previous character isn't a space and the current is a hash,
		// then this must be the end of a multi-word hash.
		case prev != ' ' && r == '#':
			end = i

		// If currently scanning a hash and a space is found without a subsequent
		// hash then this is either a multi-word hash or some unrelated text
		// so store the current position as the possible end of the hash.
		case inHash && r == ' ' && peek != '#':
			end = i
			multiWord = true

		// When a space is found followed by a hash, then this must
		// be the end of the current hash.
		case r == ' ' && peek == '#' && inHash:
			inHash = false
			multiWord = false
			hashtags = append(hashtags, formatTag(l[start:end], tag))

		// If this isn't a potential multi-word hash, then keep incrementing the end index.
		case !multiWord:
			end = i + 1
		}

		prev = rune(r)
	}

	if inHash {
		hashtags = append(hashtags, formatTag(l[start:end], tag))
	}

	return hashtags
}

func parseImages(lines [][]byte, imgDir string) {
	caption := false

	// Go through all the lines and check for images.
	// Replace the Bear image format with the Hugo format and the captions.
	for i, l := range lines {
		switch {
		case caption:
			caption = false

			// Assume captions are italics or bold.
			if bytes.HasPrefix(l, []byte("*")) {
				lines[i-1] = bytes.Replace(lines[i-1], []byte("--caption--"), bytes.Trim(l, "*"), -1)
			} else {
				lines[i-1] = bytes.Replace(lines[i-1], []byte("--caption--"), []byte(""), -1)
			}
		case bytes.Contains(l, []byte("[image:")):
			// Next line is possibly the image caption.
			caption = true
			split := bytes.Split(l, []byte("/"))
			if len(split) != 2 {
				log.Warn("Parsing image line failed")
				continue
			}

			imgName := string(bytes.TrimSuffix(bytes.TrimSpace(split[1]), []byte("]")))
			lines[i] = []byte(fmt.Sprintf("![--caption--](%s/%s)", imgDir, imgName))
		}
	}
}

func customFrontMatter(f []byte) []string {
	lines := bytes.Split(f, []byte("\n"))
	fm := []string{}

	for i, l := range lines {
		kv := bytes.Split(l, []byte(":"))

		switch {
		case i == 0:
			// First line should be dashes.
			if !bytes.Equal(l, []byte("---")) {
				return []string{}
			}

		// If this line is front matter that Bhguo controls, don't append it.
		case bhugoFrontMatter[string(kv[0])]:
			continue
		case bytes.Equal(l, []byte("---")):
			return fm
		default:
			fm = append(fm, string(l))
		}
	}

	// Should not reach this if file is formatted correctly.
	return []string{}
}

func formatTag(l []byte, tag string) string {
	return strings.Title(strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace((string(l))), "#"), tag+"/"))
}
