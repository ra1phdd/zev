package lexical

import (
	"reflect"
	"strings"
	"testing"
)

const (
	russianFileStem    = "файл"
	englishFileStem    = "file"
	englishConnectStem = "connect"
)

func TestNormalizeAndTokensUseTheDeterministicProfile(t *testing.T) {
	t.Parallel()

	if got, want := Normalize("  ЁЛКА	Ｆile  "), "елка file"; got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
	if got, want := Tokens("Ёлка, FILES и 東京"), []string{"елка", "files", "и", "東京"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokens() = %#v, want %#v", got, want)
	}
}

func TestMorphologyUsesRussianAndEnglishSnowballPerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "russian file forms", text: "файл файла файлы файлов", want: strings.Join([]string{russianFileStem, russianFileStem, russianFileStem, russianFileStem}, " ")},
		{name: "russian phrase forms", text: "резервная копия резервные копии", want: "резервн коп резервн коп"},
		{name: "english forms", text: "file files connect connected connecting connects", want: strings.Join([]string{englishFileStem, englishFileStem, englishConnectStem, englishConnectStem, englishConnectStem, englishConnectStem}, " ")},
		{name: "unknown script stays neutral", text: "東京", want: "東京"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Morphology(tt.text); got != tt.want {
				t.Fatalf("Morphology(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestStemDoesNotInventUnsupportedEnglishIrregularForms(t *testing.T) {
	t.Parallel()

	for _, word := range []string{"go", "went", "gone"} {
		if got := Stem(word); got != word {
			t.Errorf("Stem(%q) = %q, want unchanged unsupported form", word, got)
		}
	}
}

func TestDeletionKeysAreDeterministicAndBounded(t *testing.T) {
	t.Parallel()

	if got, want := DeletionKeys("file"), []string{"ile", "fle", "fie", "fil", "le", "ie", "il", "fe", "fl", "fi"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DeletionKeys(file) = %#v, want %#v", got, want)
	}
	if got := DeletionKeys("é"); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("DeletionKeys(é) = %#v, want one empty deletion key", got)
	}
	if got := DeletionKeys("abcdefghijklmnopqrstuvwxyzabcdef"); len(got) > 600 {
		t.Fatalf("DeletionKeys() returned %d keys, want at most 600", len(got))
	}
	if got := DeletionKeys("abcdefghijklmnopqrstuvwxyzabcdefg"); got != nil {
		t.Fatalf("DeletionKeys() for overlong token = %#v, want nil", got)
	}
}

func TestDistanceAndSimilarityAreUnicodeAware(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b string
		want int
	}{
		{a: "файл", b: "фалй", want: 1},
		{a: "file", b: "fiel", want: 1},
		{a: "ёлка", b: "елка", want: 0},
		{a: "kitten", b: "sitting", want: 3},
	}
	for _, tt := range cases {
		if got := Distance(tt.a, tt.b); got != tt.want {
			t.Errorf("Distance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}

	for _, tt := range []struct {
		name string
		a, b string
		want bool
	}{
		{name: "short typo rejected", a: "cat", b: "cut", want: false},
		{name: "short insertion rejected", a: "cat", b: "cats", want: false},
		{name: "four character typo accepted", a: "file", b: "fiel", want: true},
		{name: "long typo accepted", a: "terminal", b: "termi nal", want: true},
		{name: "too many long edits rejected", a: "abcdefghij", b: "abXYZefghij", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, got := Similarity(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("Similarity(%q, %q) accepted = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
	if got, accepted := Similarity("file", "file"); got != 1 || !accepted {
		t.Fatalf("Similarity exact = (%v, %v), want (1, true)", got, accepted)
	}
}

func TestTrigramsUseUnicodeCodePoints(t *testing.T) {
	t.Parallel()

	if got, want := Trigrams("Файл"), []string{"фай", "айл"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Trigrams() = %#v, want %#v", got, want)
	}
	if got, want := Trigrams("aaaa"), []string{"aaa", "aaa"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Trigrams(repeated) = %#v, want %#v", got, want)
	}
	if got := Trigrams("東京"); got != nil {
		t.Fatalf("Trigrams(short token) = %#v, want nil", got)
	}
}

func BenchmarkLexicalPrimitives(b *testing.B) {
	input := "Резервные копии файлов connect connected connecting ёлка"
	for b.Loop() {
		_ = Normalize(input)
		_ = Tokens(input)
		_ = Morphology(input)
		_ = DeletionKeys("резервные")
		_ = Distance("файл", "фалй")
		_, _ = Similarity("connected", "connectd")
		_ = Trigrams("резервные")
	}
}
