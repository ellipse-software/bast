package cloud

import "testing"

func TestFactDropsTheRemedy(t *testing.T) {
	cases := map[string]string{
		"not logged in; run fly auth login":                                        "not logged in",
		"gcloud CLI not found; install Google Cloud SDK and run gcloud auth login": "gcloud not found",
		"GCP credentials expired. Run gcloud auth login":                           "credentials expired",
		"API 403: missing vms:read":                                                "API 403: missing vms:read",
		"doctl not found; install the DigitalOcean CLI and run doctl auth init":    "doctl not found",
	}
	for in, want := range cases {
		if got := Fact(in); got != want {
			t.Errorf("Fact(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnsetIsMissingSetup(t *testing.T) {
	for _, msg := range []string{
		"not logged in; run fly auth login",
		"gcloud CLI not found; install Google Cloud SDK and run gcloud auth login",
		"no API token; connect on the Sync tab",
		"aws executable not found",
		"boat not found",
	} {
		if !Unset(msg) {
			t.Errorf("Unset(%q) = false", msg)
		}
	}
	if Unset("API 403: missing vms:read") || Unset("credentials expired") {
		t.Fatal("a real failure was treated as missing setup")
	}
}
