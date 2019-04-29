package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpdateHugo(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		in      note
		exp     []byte
		cleanup bool
	}{
		{
			"basic",
			"note-title.md",
			note{
				Title: "Note Title",
				BodyRaw: []byte(`# Note Title
#blog/tag

Body text`)},
			[]byte(`---
title: "Note Title"
date: %time%
categories: ["Tag"]
tags: ["Tag"]
draft: false
---

Body text`),
			true,
		},
		// Should preserve custom front matter of an existing note.
		{
			"existing note",
			"existing.md",
			note{
				Title: "Existing",
				BodyRaw: []byte(`# Existing
#blog/tag

Updated text`)},
			[]byte(`---
title: "Existing"
date: %time%
categories: ["Tag"]
tags: ["Tag"]
draft: false
custom: abc
---

Updated text`),
			false,
		},
	}

	now := time.Now()
	tp := func() time.Time {
		return now
	}
	tf := "2006-01-02T15:04:05-07:00"
	tag := "blog"
	hugoDir := "./testData/site"
	contentDir := "content"
	imageDir := "/"

	done := make(chan bool, 2)
	notes := make(chan note, 1)

	tmpl, err := template.New("Note Template").Parse(templateRaw)
	require.NoError(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := fmt.Sprintf("%s/%s/%s", hugoDir, contentDir, test.file)

			// Keep a copy of the original file if it exists.
			orig, _ := ioutil.ReadFile(dir)

			defer func() {
				if test.cleanup {
					err = os.Remove(dir)
					require.NoError(t, err)
				} else {
					err := ioutil.WriteFile(dir, orig, 0666)
					require.NoError(t, err)
				}
			}()

			wg := sync.WaitGroup{}
			wg.Add(1)
			go updateHugo(&wg, done, notes, tp, tf, tag, hugoDir, contentDir, imageDir, tmpl, true, true)
			notes <- test.in

			// Pause for a moment to make sure the note is processed before the done channel.
			time.Sleep(time.Millisecond)

			done <- true
			wg.Wait()

			f, err := ioutil.ReadFile(dir)
			require.NoError(t, err)

			// Replace the date placeholder with the dummy timestamp.
			exp := bytes.Replace(test.exp, []byte("%time%"), []byte(tp().Format(tf)), 1)

			require.Equal(t, string(exp), string(f))
		})
	}
}

func TestScanTags(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		exp  []string
	}{
		{
			"empty",
			[]byte(""),
			[]string{},
		},
		{
			"one tag",
			[]byte("#prefix/abc"),
			[]string{"Abc"},
		},
		{
			"multi-word tag",
			[]byte("#prefix/abc def#"),
			[]string{"Abc Def"},
		},
		{
			"multiple tags",
			[]byte("#prefix/abc #prefix/def abc#  #def"),
			[]string{"Abc", "Def Abc", "Def"},
		},
		{
			"not hashes",
			[]byte("1234"),
			[]string{},
		},
		{
			"some hashes with some random text",
			[]byte("#prefix/abc 123 #one 456"),
			[]string{"Abc", "One"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := scanTags(test.in, "prefix")
			require.Equal(t, test.exp, got)
		})
	}
}

func TestParseImages(t *testing.T) {
	tests := []struct {
		name string
		in   [][]byte
		exp  [][]byte
	}{
		{"empty", nil, nil},
		{
			"basic",
			[][]byte{
				[]byte("[image:7BD34BA7-1D41-4634-B42B-0C6D20B88E33-34561-0000B3447A4CA4D0/img.jpg]"),
				[]byte("*Caption*"),
			},
			[][]byte{
				[]byte("![Caption](/img/posts/img.jpg)"),
				[]byte("*Caption*"),
			},
		},
		{
			"no catpion",
			[][]byte{
				[]byte("[image:7BD34BA7-1D41-4634-B42B-0C6D20B88E33-34561-0000B3447A4CA4D0/img.jpg]"),
				[]byte(""),
			},
			[][]byte{
				[]byte("![](/img/posts/img.jpg)"),
				[]byte(""),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parseImages(test.in, "/img/posts")
			for i, l := range test.exp {
				require.Equal(t, string(l), string(test.in[i]))
			}
		})
	}
}

func TestCustomFrontMatter(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		exp  []string
	}{
		{"empty", nil, []string{}},
		{
			"basic",
			[]byte(`---
title: "Existing"
date: 2019-04-29T07:55:21-07:00
draft: false
categories: ["blog"]
tags: ["custom-tag"]
custom: abc
---

Body Text`),
			[]string{"tags: [\"custom-tag\"]", "custom: abc"},
		},
		{
			"no opening dash",
			[]byte(`title: "Existing"
date: 2019-04-29T07:55:21-07:00
draft: false
categories: ["blog"]
tags: ["custom-tag"]
custom: abc
---

Body Text`),
			[]string{},
		},
		{
			"no closing dash",
			[]byte(`---
title: "Existing"
date: 2019-04-29T07:55:21-07:00
draft: false
categories: ["blog"]
tags: ["custom-tag"]
custom: abc

Body Text`),
			[]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := customFrontMatter(test.in, true, false)
			require.Equal(t, test.exp, got)
		})
	}
}
