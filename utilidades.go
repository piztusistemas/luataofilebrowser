package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// rexistroLiñas pasa a saída dun proceso ao log do frontend liña a liña.
type rexistroLiñas struct{ fn func(string) }

func (w *rexistroLiñas) Write(p []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		if l = strings.TrimRight(l, "\r"); strings.TrimSpace(l) != "" {
			w.fn(l)
		}
	}
	return len(p), nil
}

func executarConRexistro(cmd *exec.Cmd, log func(string)) error {
	w := &rexistroLiñas{fn: log}
	cmd.Stdout, cmd.Stderr = w, w
	return cmd.Run()
}

// escribirIdiomaLua deixa en <config do usuario>/lua/settings.json o idioma escollido no
// instalador (a mesma ruta que le lua/settings.go). Fusiona: nunha reinstalación non se
// perde o resto de axustes (clave de IA, compilador LaTeX...).
//
// En Windows a elevación por UAC non cambia de perfil, así que os.UserConfigDir() é o
// do profesor... salvo se quen escribiu o contrasinal de administrador foi OUTRA conta:
// entón isto queda no perfil desa conta e Lúa arrinca no idioma por defecto.
func escribirIdiomaLua(idioma string) error {
	if idioma == "" {
		idioma = idiomaFallback
	}
	base, err := dirConfigUsuario()
	if err != nil {
		return fmt.Errorf("localizando o cartafol de configuración: %w", err)
	}
	dir := filepath.Join(base, "lua")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ruta := filepath.Join(dir, "settings.json")
	axustes := map[string]any{}
	if datos, err := os.ReadFile(ruta); err == nil {
		json.Unmarshal(datos, &axustes)
	}
	axustes["idioma"] = idioma
	datos, err := json.MarshalIndent(axustes, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ruta, datos, 0o644); err != nil {
		return err
	}
	axustarPropietario(dir)
	return nil
}
