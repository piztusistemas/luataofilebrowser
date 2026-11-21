//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestUnitSystemd(t *testing.T) {
	u := xerarUnit(rutasFileBrowser())
	for _, quere := range []string{
		"User=www-data",
		"ExecStart=/opt/piztu/filebrowser/filebrowser -c /etc/filebrowser/config.yaml",
		"AmbientCapabilities=CAP_NET_BIND_SERVICE",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(u, quere) {
			t.Errorf("a unit non leva %q:\n%s", quere, u)
		}
	}
}

func TestRutasLinuxNonBorranSistema(t *testing.T) {
	// desinstalarFileBrowser fai RemoveAll de DirBin: nunca pode ser un cartafol do sistema.
	if d := rutasFileBrowser().DirBin; !strings.HasPrefix(d, "/opt/piztu/") {
		t.Errorf("DirBin perigoso: %s", d)
	}
}

func TestIdAtallo(t *testing.T) {
	if got := idAtallo("Lúa"); got != "piztu-lua" {
		t.Errorf("idAtallo(Lúa) = %q", got)
	}
}
