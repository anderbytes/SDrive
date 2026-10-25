package i18n

import (
	"embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// English source text is the fallback and translation key for every locale.
//
//go:embed pt-BR.json es.json fr.json de.json it.json zh-CN.json
var catalogFiles embed.FS

var translations = func() map[string]map[string]string {
	catalogs := make(map[string]map[string]string)
	for _, language := range []string{"pt-BR", "es", "fr", "de", "it", "zh-CN"} {
		data, err := catalogFiles.ReadFile(language + ".json")
		if err != nil {
			panic(err)
		}
		var values map[string]string
		if err := json.Unmarshal(data, &values); err != nil {
			panic(err)
		}
		catalogs[language] = values
	}
	return catalogs
}()

var formatToken = regexp.MustCompile(`%[sdwv]`)

type formatTranslation struct {
	pattern *regexp.Regexp
	source  string
}

var formats = func() []formatTranslation {
	var result []formatTranslation
	sources := make([]string, 0, len(translations["pt-BR"]))
	for source := range translations["pt-BR"] {
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
		if !formatToken.MatchString(source) {
			continue
		}
		parts := formatToken.Split(source, -1)
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		result = append(result, formatTranslation{regexp.MustCompile("(?s)^" + strings.Join(parts, "(.*?)") + "$"), source})
	}
	return result
}()

func Valid(language string) bool { return language == "en" || translations[language] != nil }

func Text(language, text string) string {
	if catalog := translations[language]; catalog != nil {
		if translated, ok := catalog[text]; ok {
			return translated
		}
		for _, format := range formats {
			translated, ok := catalog[format.source]
			if !ok {
				continue
			}
			if match := format.pattern.FindStringSubmatch(text); match != nil {
				i := 0
				return formatToken.ReplaceAllStringFunc(translated, func(string) string {
					i++
					return match[i]
				})
			}
		}
	}
	return text
}
