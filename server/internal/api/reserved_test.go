package api

import "testing"

func TestReservedConnSlug(t *testing.T) {
	for _, slug := range []string{"kb", "kb-qa", "app", "data", "home", "ee"} {
		if !reservedConnSlug(slug) {
			t.Errorf("%q 应保留", slug)
		}
	}
	for _, slug := range []string{"crm", "homepage", "eec", "my-app"} {
		if reservedConnSlug(slug) {
			t.Errorf("%q 不应保留", slug)
		}
	}
}
