package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanTags(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		exp  []string
	}{
		{
			"empty",
			[]byte(""),
			nil,
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
			nil,
		},
		{
			"some hashes with some random text",
			[]byte("#prefix/abc 123 #one 456"),
			[]string{"Abc", "One"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			n := note{}
			scanTags(test.in, &n, "prefix")
			require.Equal(t, test.exp, n.Categories)
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
