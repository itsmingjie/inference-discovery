package discovery

import (
	"net"
	"strings"
	"testing"
)

func TestDescriptorURLs(t *testing.T) {
	txt := []string{"v=1", "transport=http", "path=/.well-known/inference.json"}
	urls, err := descriptorURLs("test.local.", 8080, txt, []net.IP{net.ParseIP("192.0.2.1")}, []net.IP{net.ParseIP("fe80::1")}, "en0")
	if err != nil || len(urls) != 2 || !strings.Contains(urls[1], "[fe80::1%25en0]:8080") {
		t.Fatal(urls, err)
	}
	for _, bad := range [][]string{{"v=2", "transport=http", "path=/d"}, {"v=1", "V=1", "transport=http", "path=/d"}, {"v=1", "transport=http", "path=//evil.com/x"}, {"v=1", "transport=http", "path=/d?secret=x"}, {"v=1", "transport=ftp", "path=/d"}, {"v=1", "transport=http", "path=/../x"}, {"v=1", "transport=http", "path=/" + strings.Repeat("a", 129)}} {
		if _, err = descriptorURLs("test.local.", 8080, bad, []net.IP{net.ParseIP("192.0.2.1")}, nil, "en0"); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	txt[1] = "transport=https"
	urls, err = descriptorURLs("test.local.", 443, txt, nil, nil, "en0")
	if err != nil || urls[0] != "https://test.local:443/.well-known/inference.json" {
		t.Fatal(urls, err)
	}
}
