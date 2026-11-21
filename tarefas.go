package main

// Cada botón do instalador é unha tarefaAdmin. Separalas así permite que macOS execute
// unha tarefa nun proceso aparte con permisos de administrador (ver comoAdministrador en
// plataforma_macos.go): a tarefa viaxa coma JSON e o proceso fillo chama a
// executarTarefa igual que faría a xanela. En Windows e Debian o instalador xa corre
// elevado e comoAdministrador chama a executarTarefa directamente.

import "fmt"

const (
	tarefaFBInstalar     = "fb-instalar"
	tarefaFBDesinstalar  = "fb-desinstalar"
	tarefaTaoInstalar    = "tao-instalar"
	tarefaTaoDesinstalar = "tao-desinstalar"
	tarefaLuaInstalar    = "lua-instalar"
	tarefaLuaDesinstalar = "lua-desinstalar"
)

type tarefaAdmin struct {
	Nome        string `json:"nome"`
	Idioma      string `json:"idioma"`
	Usuario     string `json:"usuario,omitempty"`     // fb-instalar
	Contrasinal string `json:"contrasinal,omitempty"` // fb-instalar
	BorrarDatos bool   `json:"borrarDatos,omitempty"` // fb-desinstalar, tao-desinstalar
	Deps        bool   `json:"deps,omitempty"`        // lua-*: instalar ou desinstalar LaTeX e Pandoc
}

// executarTarefa fai a tarefa neste mesmo proceso. Devolve os datos extra do evento
// <tarefa>_completo (o porto e as URLs, para File Browser).
func executarTarefa(tr *Translator, t tarefaAdmin, log func(string)) (map[string]any, error) {
	switch t.Nome {
	case tarefaFBInstalar:
		porto, err := instalarFileBrowser(tr, t.Usuario, t.Contrasinal, log)
		if err != nil {
			return nil, err
		}
		urls := urlsAlumnado(porto)
		log(tr.T("fb.log.urls"))
		for _, u := range urls {
			log("   " + u)
		}
		return map[string]any{"porto": porto, "urls": urls}, nil
	case tarefaFBDesinstalar:
		return nil, desinstalarFileBrowser(tr, t.BorrarDatos, log)
	case tarefaTaoInstalar:
		return nil, instalarTao(tr, log)
	case tarefaTaoDesinstalar:
		return nil, desinstalarTao(tr, t.BorrarDatos, log)
	case tarefaLuaInstalar:
		return nil, instalarLua(tr, t.Idioma, t.Deps, log)
	case tarefaLuaDesinstalar:
		return nil, desinstalarLua(tr, t.Deps, log)
	}
	return nil, fmt.Errorf("tarefa descoñecida: %q", t.Nome)
}
