package i18n

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// English source text is the fallback and translation key for every locale.
//
//go:embed pt-BR.json
var portuguese []byte

var translations = func() map[string]string {
	var values map[string]string
	if err := json.Unmarshal(portuguese, &values); err != nil {
		panic(err)
	}
	return values
}()

var formatToken = regexp.MustCompile(`%[sdwv]`)

type formatTranslation struct {
	pattern *regexp.Regexp
	value   string
}

var formats = func() []formatTranslation {
	var result []formatTranslation
	sources := make([]string, 0, len(translations))
	for source := range translations {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		left, right := formatToken.ReplaceAllString(sources[i], ""), formatToken.ReplaceAllString(sources[j], "")
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return sources[i] < sources[j]
	})
	for _, source := range sources {
		translated := translations[source]
		if !formatToken.MatchString(source) {
			continue
		}
		parts := formatToken.Split(source, -1)
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		result = append(result, formatTranslation{regexp.MustCompile("(?s)^" + strings.Join(parts, "(.*?)") + "$"), translated})
	}
	return result
}()

func Valid(language string) bool { return language == "en" || language == "pt-BR" }

func Text(language, text string) string {
	if language == "pt-BR" {
		if translated, ok := translations[text]; ok {
			return translated
		}
		for _, format := range formats {
			if match := format.pattern.FindStringSubmatch(text); match != nil {
				i := 0
				return formatToken.ReplaceAllStringFunc(format.value, func(string) string {
					i++
					return match[i]
				})
			}
		}
	}
	return text
}
