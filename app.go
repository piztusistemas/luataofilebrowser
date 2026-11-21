package main

import (
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App expón ao frontend os pasos do instalador (bindings xerados por
// `wails3 generate bindings` en frontend/bindings/) e emite o progreso por eventos:
// <tarefa>_log con cada liña e <tarefa>_completo con {ok, erro, ...} ao rematar.
type App struct {
	tr *Translator

	mu      sync.Mutex
	ocupado bool // unha tarefa á vez: dúas instalacións en paralelo pisaríanse
}

func NewApp() *App { return &App{tr: NewTranslator(idiomaFallback)} }

// emitir envía un evento ao frontend (no-op fóra de Wails, nos tests).
func emitir(nome string, datos any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(nome, datos)
	}
}

// tarefa executa fn en segundo plano e informa do progreso co prefixo de evento dado.
// Devolve false se xa había outra en marcha.
func (a *App) tarefa(evento string, fn func(log func(string)) (map[string]any, error)) bool {
	a.mu.Lock()
	if a.ocupado {
		a.mu.Unlock()
		return false
	}
	a.ocupado = true
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.ocupado = false
			a.mu.Unlock()
		}()
		log := func(l string) { emitir(evento+"_log", strings.TrimRight(l, "\r\n")) }
		extra, err := fn(log)
		res := map[string]any{"ok": err == nil, "erro": ""}
		for k, v := range extra {
			res[k] = v
		}
		if err != nil {
			res["erro"] = a.tr.T("erro.xeral", err.Error())
			log(res["erro"].(string))
		} else {
			log(a.tr.T("ok.xeral"))
		}
		emitir(evento+"_completo", res)
	}()
	return true
}

// ── Idioma e información ─────────────────────────────────────────────────────

func (a *App) Idioma() string                { return a.tr.Idioma() }
func (a *App) SetIdioma(code string)         { a.tr.SetIdioma(code) }
func (a *App) Idiomas() []IdiomaInfo         { return idiomasDispo }
func (a *App) Traducions() map[string]string { return a.tr.All() }
func (a *App) Version() string               { return strings.TrimSpace(versionInstalador) }
func (a *App) Plataforma() string            { return runtime.GOOS }

// Estado resume o que xa hai instalado neste equipo, para a benvida e o final.
type Estado struct {
	FileBrowser bool     `json:"fileBrowser"`
	Porto       int      `json:"porto"`
	URLs        []string `json:"urls"`
	Tao         bool     `json:"tao"`
	Lua         bool     `json:"lua"`
	DirTao      string   `json:"dirTao"`
	DirLua      string   `json:"dirLua"`
	DirArquivos string   `json:"dirArquivos"`
	Rexistro    string   `json:"rexistro"`
}

func (a *App) Estado() Estado {
	r := rutasFileBrowser()
	p := portoInstalado()
	e := Estado{
		FileBrowser: p != 0,
		Porto:       p,
		Tao:         instalado(dirTao(), nomeBinTao),
		Lua:         instalado(dirLua(), nomeBinLua),
		DirTao:      raizApp(dirTao()),
		DirLua:      raizApp(dirLua()),
		DirArquivos: r.Raiz,
		Rexistro:    r.Rexistro,
	}
	if p != 0 {
		e.URLs = urlsAlumnado(p)
	}
	return e
}

// ── Instalación ──────────────────────────────────────────────────────────────

// Cada botón é unha tarefaAdmin (tarefas.go). comoAdministrador (plataforma_<so>.go)
// execútaa aquí mesmo en Windows e Debian, onde o instalador xa corre elevado, e en
// macOS nun proceso aparte con permisos de administrador só cando fai falta.

func (a *App) executar(evento string, t tarefaAdmin) bool {
	t.Idioma = a.tr.Idioma()
	return a.tarefa(evento, func(log func(string)) (map[string]any, error) {
		return comoAdministrador(a.tr, t, log)
	})
}

func (a *App) InstalarFileBrowser(usuario, contrasinal string) bool {
	return a.executar("fb", tarefaAdmin{Nome: tarefaFBInstalar,
		Usuario: strings.TrimSpace(usuario), Contrasinal: contrasinal})
}

func (a *App) InstalarTao() bool {
	return a.executar("tao", tarefaAdmin{Nome: tarefaTaoInstalar})
}

func (a *App) InstalarLua(conDeps bool) bool {
	return a.executar("lua", tarefaAdmin{Nome: tarefaLuaInstalar, Deps: conDeps})
}

// ── Desinstalación ───────────────────────────────────────────────────────────

func (a *App) DesinstalarFileBrowser(borrarDatos bool) bool {
	return a.executar("desinstfb", tarefaAdmin{Nome: tarefaFBDesinstalar, BorrarDatos: borrarDatos})
}

func (a *App) DesinstalarTao(borrarDatos bool) bool {
	return a.executar("desinsttao", tarefaAdmin{Nome: tarefaTaoDesinstalar, BorrarDatos: borrarDatos})
}

func (a *App) DesinstalarLua(borrarDeps bool) bool {
	return a.executar("desinstlua", tarefaAdmin{Nome: tarefaLuaDesinstalar, Deps: borrarDeps})
}

