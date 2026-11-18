package version

import "testing"

func TestFromModule(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"v0.1.0", "0.1.0", true},
		{"v1.2.3-rc.1", "1.2.3-rc.1", true},
		{"(devel)", "", false},
		{"", "", false},
		{"v0.0.0-20261118150000-0c76d33abcde", "", false},
		{"v0.1.1-0.20261118150000-0c76d33abcde", "", false},
		{"0.1.0", "", false},
	} {
		got, ok := fromModule(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("fromModule(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSyntheticImage(t *testing.T) {
	defer func(i string) { Image = i }(Image)
	Image = ""
	if got := SyntheticImage(); got != DevImage {
		t.Errorf("no release image: got %q, want %q", got, DevImage)
	}
	Image = Published + ":0.1.0"
	if got := SyntheticImage(); got != Published+":0.1.0" {
		t.Errorf("release image: got %q", got)
	}
	if Pullable(DevImage) || Pullable("loadlore/synthetic:2d0ae1e") || !Pullable(Published+":0.1.0") || !Pullable("ghcr.io/me/api:1") {
		t.Error("Pullable: only images make image builds need building")
	}
}
