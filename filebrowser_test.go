package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rutasProba() fbRutas {
	return fbRutas{
		DB:    `C:\ProgramData\Piztu\FileBrowser\filebrowser.db`,
		Cache: `C:\ProgramData\Piztu\FileBrowser\cache`,
		Raiz:  `C:\ProgramData\Piztu\arquivos`,
	}
}

func TestConfigQuantumLevaOEsencial(t *testing.T) {
	cfg := xerarConfigQuantum(rutasProba(), 80, "admin", `con"comiñas\e ñ`)
	for _, quere := range []string{
		"  port: 80\n",
		`  baseURL: "/arquivos"` + "\n",
		`      name: "arquivos"` + "\n",
		"        useLogicalSize: true\n",
		`  adminUsername: "admin"` + "\n",
		// Barras invertidas de Windows escapadas: senón YAML lería \P, \F... coma escapes.
		`    - path: "C:\\ProgramData\\Piztu\\arquivos"` + "\n",
		// Comiñas e barras do contrasinal escapadas.
		`  adminPassword: "con\"comiñas\\e ñ"` + "\n",
	} {
		if !strings.Contains(cfg, quere) {
			t.Errorf("falta %q en:\n%s", quere, cfg)
		}
	}
}

func TestQuitarAdminPasswordDeixaOResto(t *testing.T) {
	con := xerarConfigQuantum(rutasProba(), 8080, "admin", "contrasinal-longo")
	sen := quitarAdminPassword(con)
	if strings.Contains(sen, "adminPassword") || strings.Contains(sen, "contrasinal-longo") {
		t.Fatalf("o contrasinal segue no config:\n%s", sen)
	}
	if sen != xerarConfigQuantum(rutasProba(), 8080, "admin", "") {
		t.Errorf("quitar o contrasinal cambiou máis cousas:\n%s", sen)
	}
	if p := portoDoConfig(sen); p != 8080 {
		t.Errorf("porto = %d", p)
	}
}

func TestValidarCredenciais(t *testing.T) {
	casos := []struct {
		u, c string
		ok   bool
	}{
		{"admin", "doce-caracteres", true},
		{"admin", "curto", false},
		{"ad min", "doce-caracteres", false},
		{"P@ss!word", "doce-caracteres", false}, // o contrasinal escrito no campo do usuario
		{"", "doce-caracteres", false},
		{"admin", "ñññññññññññ", false}, // 11 runas, aínda que sexan 22 bytes
	}
	for _, c := range casos {
		if err := validarCredenciaisFB(c.u, c.c); (err == nil) != c.ok {
			t.Errorf("validar(%q, %q) = %v", c.u, c.c, err)
		}
	}
}

func TestURLPublica(t *testing.T) {
	if u := urlPublicaFB("profe.local", 80); u != "http://profe.local/arquivos" {
		t.Errorf("porto 80: %s", u)
	}
	if u := urlPublicaFB("10.0.0.5", 8080); u != "http://10.0.0.5:8080/arquivos" {
		t.Errorf("porto 8080: %s", u)
	}
}

func TestPasswordHeaderComoTao(t *testing.T) {
	if h := fbPasswordHeader("a b+ñ%"); h != "a%20b%2B%C3%B1%25" {
		t.Errorf("cabeceira = %s", h)
	}
}

func TestActualizarAdminTaoConservaOResto(t *testing.T) {
	dir := t.TempDir()
	if feito, err := actualizarAdminTao(dir, "admin", "novo"); feito || err != nil {
		t.Fatalf("sen admin.json non debería facer nada: %v %v", feito, err)
	}
	os.MkdirAll(filepath.Join(dir, "dixitalizacion"), 0o755)
	ruta := filepath.Join(dir, "dixitalizacion", "admin.json")
	os.WriteFile(ruta, []byte(`{"center":"IES","fbAdminUser":"vello","fbAdminPass":"vello"}`), 0o600)
	if feito, err := actualizarAdminTao(dir, "admin", "novo"); !feito || err != nil {
		t.Fatalf("feito=%v err=%v", feito, err)
	}
	var cfg map[string]string
	datos, _ := os.ReadFile(ruta)
	json.Unmarshal(datos, &cfg)
	if cfg["center"] != "IES" || cfg["fbAdminUser"] != "admin" || cfg["fbAdminPass"] != "novo" {
		t.Errorf("admin.json = %v", cfg)
	}
}

func TestEscollerPortoSaltaOcupado(t *testing.T) {
	ocupado, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skip("non se pode abrir un porto de proba:", err)
	}
	defer ocupado.Close()
	pOcupado := ocupado.Addr().(*net.TCPAddr).Port

	libre, _ := net.Listen("tcp", ":0")
	pLibre := libre.Addr().(*net.TCPAddr).Port
	libre.Close()

	p, err := escollerPorto([]int{pOcupado, pLibre})
	if err != nil || p != pLibre {
		t.Fatalf("escollerPorto = %d, %v; quería %d", p, err, pLibre)
	}
	if _, err := escollerPorto([]int{pOcupado}); err == nil {
		t.Error("cun só porto ocupado debería dar erro")
	}
}
