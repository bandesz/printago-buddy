package smartqueue

import (
	"errors"
	"strings"
)

// ErrUserTags is returned when user.tags is not empty and is not an &= list.
var ErrUserTags = errors.New("user.tags must be empty or an &= list")

const (
	userTagsKey  = "user.tags"
	printerIDKey = "printer.id"
	parkTag      = "buddy-hold"
	andPrefix    = "&="
)

func Park(tags map[string]string) (map[string]string, error) {
	return rewrite(tags, func(list []string) []string {
		return ensureTag(list, parkTag)
	}, func(out map[string]string) {
		delete(out, printerIDKey)
	})
}

func Pin(tags map[string]string, printerID string) (map[string]string, error) {
	return rewrite(tags, func(list []string) []string {
		return removeTag(list, parkTag)
	}, func(out map[string]string) {
		out[printerIDKey] = printerID
	})
}

func ClearTags(tags map[string]string) (map[string]string, error) {
	return rewrite(tags, func(list []string) []string {
		return removeTag(list, parkTag)
	}, func(out map[string]string) {
		delete(out, printerIDKey)
	})
}

func rewrite(tags map[string]string, edit func([]string) []string, printer func(map[string]string)) (map[string]string, error) {
	list, err := parseAndList(tags[userTagsKey])
	if err != nil {
		return nil, err
	}
	list = edit(list)
	out := cloneTags(tags)
	if formatted, ok := formatAndList(list); ok {
		out[userTagsKey] = formatted
	} else {
		delete(out, userTagsKey)
	}
	printer(out)
	return out, nil
}

func ensureTag(list []string, tag string) []string {
	for _, item := range list {
		if item == tag {
			return list
		}
	}
	return append(list, tag)
}

func removeTag(list []string, tag string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item != tag {
			out = append(out, item)
		}
	}
	return out
}

func formatAndList(list []string) (string, bool) {
	if len(list) == 0 {
		return "", false
	}
	return andPrefix + strings.Join(list, ","), true
}

func parseAndList(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, andPrefix) {
		return nil, ErrUserTags
	}
	body := strings.TrimPrefix(raw, andPrefix)
	if body == "" {
		return nil, nil
	}
	return strings.Split(body, ","), nil
}

func cloneTags(tags map[string]string) map[string]string {
	out := make(map[string]string, len(tags)+1)
	for k, v := range tags {
		out[k] = v
	}
	return out
}
