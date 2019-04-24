package main

import (
	"bytes"
	"fmt"
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
	Title      string `db:"ZTITLE"`
	BodyRaw    []byte `db:"ZTEXT"`
	Body       string
	Date       string
	Categories []string
	Draft      bool
}

var templateRaw = `---
title: "{{ .Title }}"
date: {{ .Date }}
categories: [ 
{{- range $i, $c := .Categories -}}
	{{- if $i -}},{{- end -}}
	"{{- $c -}}"
{{- end -}}
]
draft: {{ .Draft }}
---
{{ .Body }}`

func main() {
	log.Info("Bhugo Initializing")

	err := godotenv.Load(".bhugo")
	if err != nil {
		log.Fatal(err)
	}

	var cfg struct {
		Interval   int    `default:"1"`
		HugoDir    string `split_words:"true" required:"true"`
		ContentDir string `split_words:"true" default:"content/blog"`
		ImageDir   string `split_words:"true" default:"/img/posts"`
		NoteTag    string `split_words:"true" default:"blog"`
		Database   string `required:"true"`
	}

	err = envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err)
	}

	timeFormat := "2006-01-02T15:04:05-07:00"
	interval := time.Duration(cfg.Interval) * time.Second

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
	go checkBear(&wg, done, db, interval, notes, cfg.NoteTag)

	wg.Add(1)
	go updateHugo(&wg, done, notes, timeFormat, cfg.NoteTag, cfg.HugoDir, cfg.ContentDir, cfg.ImageDir, tmpl)

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

func updateHugo(wg *sync.WaitGroup, done <-chan bool, notes <-chan note, timeFormat, noteTag, hugoDir, contentDir, imageDir string, tmpl *template.Template) {
	log.Debug("Starting UpdateHugo")
	defer wg.Done()

	for {
		select {
		case n := <-notes:
			// Replace smart quotes with regular quotes.
			n.BodyRaw = bytes.Replace(n.BodyRaw, []byte("“"), []byte("\""), -1)
			n.BodyRaw = bytes.Replace(n.BodyRaw, []byte("”"), []byte("\""), -1)

			n.Date = time.Now().Format(timeFormat)

			lines := bytes.Split(n.BodyRaw, []byte("\n"))
			// If there is only a heading and tags continue on.
			if len(lines) < 3 {
				continue
			}

			// The second line should be the line with tags.
			scanTags(lines[1], &n, noteTag)

			for _, c := range n.Categories {
				if strings.Contains(strings.ToLower(c), "draft") {
					n.Draft = true
				}
			}

			// Format images for Hugo.
			parseImages(lines, imageDir)

			// First two lines are the title of the note and the tags.
			n.Body = string(bytes.Join(lines[2:], []byte("\n")))
			target := strings.Replace(strings.ToLower(n.Title), " ", "-", -1)

			f, err := os.Create(fmt.Sprintf("%s/%s/%s.md", hugoDir, contentDir, target))
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

func scanTags(l []byte, n *note, tag string) {
	start := 0
	end := 0
	inHash := false
	multiWord := false
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
			n.Categories = append(n.Categories, formatTag(l[start:end], tag))

		// If this isn't a potential multi-word hash, then keep incrementing the end index.
		case !multiWord:
			end = i + 1
		}

		prev = rune(r)
	}

	if inHash {
		n.Categories = append(n.Categories, formatTag(l[start:end], tag))
	}
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

func formatTag(l []byte, tag string) string {
	return strings.Title(strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace((string(l))), "#"), tag+"/"))
}
