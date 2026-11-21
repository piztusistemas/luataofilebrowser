package main

// Pasos de instalación e desinstalación. Son comúns a todos os sistemas: o que cambia
// dun sistema a outro (onde van os ficheiros, como se crea un servizo, o firewall, os
// accesos directos) está detrás das funcións de plataforma_<so>.go.

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// files/ éncheo build-windows.ps1: os binarios de File Browser Quantum, Tao e Lúa
// descargados das súas Releases co SHA-256 comprobado, e o envoltorio de servizo
// (cmd/fbservizo). Vai embebido: a instalación non depende de Internet (agás as
// dependencias de Lúa, que veñen de winget).
//
//go:embed files
var ficheirosEmbebidos embed.FS

// lerEmbebido devolve un ficheiro de files/ ou un erro claro se o instalador se
// compilou sen el.
func lerEmbebido(nome string) ([]byte, error) {
	datos, err := ficheirosEmbebidos.ReadFile("files/" + nome)
	if err != nil {
		return nil, fmt.Errorf("este instalador non leva %s (compilouse sen executar build-windows.ps1?)", nome)
	}
	return datos, nil
}

// escribirEmbebido copia files/<nome> a destino. Reintenta uns segundos porque, tras
// deter un servizo ou pechar un programa, Windows pode tardar en soltar o executable.
func escribirEmbebido(nome, destino string) error {
	datos, err := lerEmbebido(nome)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
		return err
	}
	var ultimo error
	for i := 0; i < 10; i++ {
		if ultimo = os.WriteFile(destino, datos, 0o755); ultimo == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("non se puido escribir %s (está aberto?): %w", destino, ultimo)
}

// ── File Browser Quantum ─────────────────────────────────────────────────────

// instalarFileBrowser é o porte do playbook de Piztu. log recibe o progreso.
func instalarFileBrowser(tr *Translator, usuario, contrasinal string, log func(string)) (int, error) {
	if err := validarCredenciaisFB(usuario, contrasinal); err != nil {
		return 0, err
	}
	r := rutasFileBrowser()

	log(tr.T("fb.log.detendo"))
	detenerServizoFB() // pode non existir aínda: non é un erro

	log(tr.T("fb.log.porto"))
	porto, err := escollerPorto(fbPortosCandidatos)
	if err != nil {
		return 0, err
	}
	log(tr.T("fb.log.portoescollido", porto))

	log(tr.T("fb.log.binarios", fbVersion, r.DirBin))
	if err := escribirEmbebido(nomeBinFB, r.Bin); err != nil {
		return 0, err
	}
	if err := escribirBinariosServizoFB(r); err != nil {
		return 0, err
	}

	// Reinstalar parte de cero (paso 3 do playbook): contas, regras e config. Os
	// ficheiros do alumnado (r.Raiz) NON se tocan.
	log(tr.T("fb.log.purgando"))
	for _, p := range []string{r.DB, r.Config, r.Estado, r.Cache} {
		if err := os.RemoveAll(p); err != nil {
			return 0, fmt.Errorf("borrando %s: %w", p, err)
		}
	}
	for _, d := range []string{filepath.Dir(r.Config), r.Estado, r.Cache, r.Raiz} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return 0, err
		}
	}
	leame := filepath.Join(r.Raiz, "leame.txt")
	if _, err := os.Stat(leame); os.IsNotExist(err) {
		os.WriteFile(leame, []byte("Benvido a File Browser.\n"), 0o644)
	}

	log(tr.T("fb.log.config"))
	if err := os.WriteFile(r.Config, []byte(xerarConfigQuantum(r, porto, usuario, contrasinal)), 0o600); err != nil {
		return 0, err
	}
	if err := protexerFicheirosFB(r); err != nil {
		return 0, fmt.Errorf("axustando permisos: %w", err)
	}

	log(tr.T("fb.log.servizo"))
	if err := instalarServizoFB(r); err != nil {
		return 0, fmt.Errorf("creando o servizo: %w", err)
	}

	base := baseLocalFB(porto)
	log(tr.T("fb.log.agardando"))
	if err := agardarSaudeFB(base, 30, 2*time.Second); err != nil {
		return 0, fmt.Errorf("%w (mira %s)", err, r.Rexistro)
	}
	log(tr.T("fb.log.login"))
	if err := comprobarLoginFB(base, usuario, contrasinal, 15); err != nil {
		return 0, err
	}

	// Pasos 11–11c: fóra o contrasinal do disco e comprobar que segue a entrar.
	log(tr.T("fb.log.quitarpass"))
	if err := os.WriteFile(r.Config, []byte(xerarConfigQuantum(r, porto, usuario, "")), 0o600); err != nil {
		return 0, err
	}
	if err := reiniciarServizoFB(); err != nil {
		return 0, fmt.Errorf("reiniciando o servizo: %w", err)
	}
	if err := agardarSaudeFB(base, 30, 2*time.Second); err != nil {
		return 0, err
	}
	if err := comprobarLoginFB(base, usuario, contrasinal, 15); err != nil {
		return 0, fmt.Errorf("tras quitar o contrasinal do config: %w", err)
	}

	log(tr.T("fb.log.firewall", porto))
	if err := abrirFirewallFB(porto); err != nil {
		log(tr.T("fb.log.firewallfail", err.Error()))
	}

	// Tao: servidor por defecto e credenciais de admin ao día.
	if err := escribirServidorTao(dirTao(), porto); err != nil {
		log("⚠️ tao-servidor.conf: " + err.Error())
	}
	if dir, err := os.UserConfigDir(); err == nil {
		if feito, err := actualizarAdminTao(dir, usuario, contrasinal); err != nil {
			log(tr.T("fb.log.admintaofail", err.Error()))
		} else if feito {
			log(tr.T("fb.log.admintao"))
		}
	}
	return porto, nil
}

// portoInstalado devolve o porto de File Browser se está instalado neste equipo (0 se non).
func portoInstalado() int {
	datos, err := os.ReadFile(rutasFileBrowser().Config)
	if err != nil {
		return 0
	}
	return portoDoConfig(string(datos))
}

// desinstalarFileBrowser quita o servizo, a regra do firewall, os binarios, a config e
// a base de datos. Os ficheiros do alumnado só se borran con borrarDatos.
func desinstalarFileBrowser(tr *Translator, borrarDatos bool, log func(string)) error {
	r := rutasFileBrowser()
	log(tr.T("desinst.fb.log.servizo"))
	if err := eliminarServizoFB(); err != nil {
		log("⚠️ " + err.Error())
	}
	log(tr.T("desinst.fb.log.firewall"))
	if err := pecharFirewallFB(); err != nil {
		log("⚠️ " + err.Error())
	}
	borrar := []string{r.DirBin, r.DB, r.Config, r.Estado, r.Cache, r.Rexistro}
	if borrarDatos {
		borrar = append(borrar, r.Raiz)
	}
	for _, p := range borrar {
		log("  🗑️ " + p)
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("borrando %s: %w", p, err)
		}
	}
	// O cartafol común de datos só se borra se quedou baleiro.
	os.Remove(filepath.Dir(r.Config))
	if !borrarDatos {
		log(tr.T("desinst.fb.log.datosquedan", r.Raiz))
	}
	return nil
}

// ── Tao ──────────────────────────────────────────────────────────────────────

func instalarTao(tr *Translator, log func(string)) error {
	dir := dirTao()
	exe := filepath.Join(dir, nomeBinTao)
	log(tr.T("tao.log.copiando", dir))
	if err := escribirEmbebido(nomeBinTao, exe); err != nil {
		return err
	}
	if p := portoInstalado(); p != 0 {
		if err := escribirServidorTao(dir, p); err != nil {
			log("⚠️ tao-servidor.conf: " + err.Error())
		} else {
			log(tr.T("tao.log.servidor", urlPublicaFB("localhost", p)))
		}
	} else {
		log(tr.T("tao.log.senfb"))
	}
	log(tr.T("log.atallos"))
	if err := crearAtallo("Tao", exe); err != nil {
		log(tr.T("log.atallosfail", err.Error()))
	}
	return nil
}

func desinstalarTao(tr *Translator, borrarDatos bool, log func(string)) error {
	log(tr.T("log.atallosborrar"))
	borrarAtallo("Tao")
	dir := dirTao()
	log("  🗑️ " + dir)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("borrando %s (está Tao aberto?): %w", dir, err)
	}
	if borrarDatos {
		// Datos locais de Tao DESTE usuario: admin.json (co contrasinal de admin en
		// claro), a CA, os certificados e as copias. Os doutros usuarios do equipo
		// quedan nos seus perfís.
		if base, err := os.UserConfigDir(); err == nil {
			datos := filepath.Join(base, "dixitalizacion")
			log("  🗑️ " + datos)
			if err := os.RemoveAll(datos); err != nil {
				return err
			}
		}
	}
	return nil
}

// ── Lúa ──────────────────────────────────────────────────────────────────────

func instalarLua(tr *Translator, idioma string, conDeps bool, log func(string)) error {
	dir := dirLua()
	exe := filepath.Join(dir, nomeBinLua)
	log(tr.T("lua.log.copiando", dir))
	if err := escribirEmbebido(nomeBinLua, exe); err != nil {
		return err
	}
	if err := escribirIdiomaLua(idioma); err != nil {
		log("⚠️ " + err.Error())
	}
	if conDeps {
		log(tr.T("lua.log.deps"))
		if err := instalarDepsLua(log); err != nil {
			// Non aborta: Lúa funciona sen LaTeX (agás xerar PDF) e en Opcións pode
			// reintentalo.
			log(tr.T("lua.log.depsfail", err.Error()))
		} else {
			log(tr.T("lua.log.depsok"))
		}
	}
	log(tr.T("log.atallos"))
	if err := crearAtallo("Lúa", exe); err != nil {
		log(tr.T("log.atallosfail", err.Error()))
	}
	return nil
}

func desinstalarLua(tr *Translator, borrarDeps bool, log func(string)) error {
	log(tr.T("log.atallosborrar"))
	borrarAtallo("Lúa")
	if borrarDeps {
		if err := desinstalarDepsLua(log); err != nil {
			log("⚠️ " + err.Error())
		}
	}
	dir := dirLua()
	log("  🗑️ " + dir)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("borrando %s (está Lúa aberta?): %w", dir, err)
	}
	return nil
}

// instalado indica se un executable está no seu sitio (para o estado da benvida).
func instalado(dir, exe string) bool {
	_, err := os.Stat(filepath.Join(dir, exe))
	return err == nil
}

var _ fs.FS = ficheirosEmbebidos
