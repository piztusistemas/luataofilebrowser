package main

// Instalación de File Browser QUANTUM (https://github.com/gtsteffaniak/filebrowser),
// porte a Go do playbook de Piztu (Piztu/playbooks/instalar_filebrowser.yaml). Este
// ficheiro ten só a parte COMÚN a todos os sistemas: o config.yaml de Quantum, o ciclo
// do contrasinal do admin, as comprobacións pola API e a elección do porto. O que
// depende do sistema (servizo, permisos, firewall) vai en plataforma_<so>.go, para que
// o instalador de Debian poida reutilizar todo isto.
//
// Ciclo do contrasinal (o mesmo do playbook, e polo mesmo motivo):
//   1. config.yaml leva auth.adminPassword: Quantum crea o admin con el no 1º arranque.
//   2. Compróbase que o admin entra (POST /api/auth/login).
//   3. Quítase adminPassword do config.yaml: mentres exista, Quantum RESTABLECE o
//      contrasinal en cada arranque, e ademais quedaría en claro no disco.
//   4. Reinicio e nova comprobación: o admin segue a entrar co hash da base de datos.
//
// Sen proxy diante (en Windows non fai falta Apache: non hai portos privilexiados e
// Quantum escoita el mesmo no 80) cada alumno chega coa súa IP, así que o límite de
// logins por IP de Quantum (~8 de golpe, despois 429) xa non xunta toda a aula nun só
// cliente coma pasaba detrás de Apache.

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// Versión fixada: a mesma que instala o playbook de Piztu en Debian, contra a que
	// se probaron Tao e o historial. O SHA-256 do binario compróbase ao compilar o
	// instalador (build-windows.ps1), que o embebe en files/.
	fbVersion = "v1.5.6-stable"

	// Subruta pública. Ten que coincidir con Tao (servidor.go: rutaArquivos) e co
	// server.baseURL do config.yaml.
	fbBaseURL = "/arquivos"

	// Nome da "source" de Quantum: é o source= de TODAS as chamadas á API de Tao
	// (tao/filebrowser.go: fbSource). Non cambialo.
	fbSource = "arquivos"
)

// fbPortosCandidatos: primeiro o 80 (o alumnado escribe http://servidor/arquivos) e,
// se está ocupado (IIS, outro servidor web...), o 8080 de fábrica. Tao proba os dous
// (tao/servidor.go: candidatasDeHosts), así que calquera dos dous funciona sen tocar
// os equipos do alumnado.
var fbPortosCandidatos = []int{80, 8080}

var reUsuarioFB = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validarCredenciaisFB repite no lado Go as comprobacións do formulario: non se debe
// poder crear unha conta admin cun contrasinal curto aínda que alguén chame ao
// binding directamente.
func validarCredenciaisFB(usuario, contrasinal string) error {
	if usuario == "" || contrasinal == "" {
		return fmt.Errorf("faltan o usuario ou o contrasinal")
	}
	if !reUsuarioFB.MatchString(usuario) {
		return fmt.Errorf("o usuario só pode levar letras, díxitos, punto, guión e guión baixo")
	}
	if len([]rune(contrasinal)) < 12 {
		return fmt.Errorf("o contrasinal ten que ter polo menos 12 caracteres")
	}
	return nil
}

// fbRutas son os ficheiros e cartafoles da instalación de File Browser. Cada sistema
// enche os seus (rutasFileBrowser en plataforma_<so>.go).
type fbRutas struct {
	DirBin  string // binarios (filebrowser e, en Windows, o envoltorio de servizo)
	Bin     string // executable de Quantum
	Config  string // config.yaml de Quantum
	DB      string // base de datos (contas, hash do contrasinal)
	Estado  string // cartafol de traballo de Quantum
	Cache   string // miniaturas e índices
	Raiz    string // os ficheiros do alumnado (a "source" arquivos)
	Rexistro string // saída do servizo
}

// cadeaYAML devolve s como cadea YAML válida sexan cales sexan os seus caracteres: un
// literal JSON é tamén YAML (mesma idea ca o to_json do playbook). Vale para
// contrasinais con comiñas e para rutas de Windows coas súas barras invertidas.
func cadeaYAML(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// xerarConfigQuantum devolve o config.yaml de Quantum. Con contrasinal != "" leva
// auth.adminPassword (só para o primeiro arranque, ver quitarAdminPassword).
func xerarConfigQuantum(r fbRutas, porto int, usuario, contrasinal string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Xerado polo instalador de aula de Piztu. Non editar a man:\n")
	fmt.Fprintf(&b, "# sobrescríbese en cada instalación de File Browser.\n")
	fmt.Fprintf(&b, "server:\n")
	fmt.Fprintf(&b, "  port: %d\n", porto)
	fmt.Fprintf(&b, "  baseURL: %s\n", cadeaYAML(fbBaseURL))
	fmt.Fprintf(&b, "  database: %s\n", cadeaYAML(r.DB))
	fmt.Fprintf(&b, "  cacheDir: %s\n", cadeaYAML(r.Cache))
	fmt.Fprintf(&b, "  logging:\n")
	fmt.Fprintf(&b, "    - output: \"stdout\"\n")
	fmt.Fprintf(&b, "  sources:\n")
	fmt.Fprintf(&b, "    - path: %s\n", cadeaYAML(r.Raiz))
	fmt.Fprintf(&b, "      name: %s\n", cadeaYAML(fbSource))
	fmt.Fprintf(&b, "      config:\n")
	// Obrigatorio: sen isto os tamaños saen redondeados a bloques de 4 KB e Tao
	// (fbXaSubido) non pode confirmar que un ficheiro xa subiu.
	fmt.Fprintf(&b, "        useLogicalSize: true\n")
	fmt.Fprintf(&b, "auth:\n")
	fmt.Fprintf(&b, "  adminUsername: %s\n", cadeaYAML(usuario))
	if contrasinal != "" {
		fmt.Fprintf(&b, "  adminPassword: %s\n", cadeaYAML(contrasinal))
	}
	return b.String()
}

// quitarAdminPassword elimina a liña auth.adminPassword dun config.yaml xerado por
// xerarConfigQuantum (o equivalente ao lineinfile do paso 11 do playbook).
func quitarAdminPassword(cfg string) string {
	liñas := strings.Split(cfg, "\n")
	saida := liñas[:0]
	for _, l := range liñas {
		if strings.HasPrefix(strings.TrimSpace(l), "adminPassword:") {
			continue
		}
		saida = append(saida, l)
	}
	return strings.Join(saida, "\n")
}

// portoDoConfig le server.port do config.yaml instalado (0 se non se atopa). Así o
// instalador sabe en que porto quedou File Browser sen gardar o dato noutro sitio.
func portoDoConfig(cfg string) int {
	for _, l := range strings.Split(cfg, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "port:") {
			if p, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(t, "port:"))); err == nil {
				return p
			}
		}
	}
	return 0
}

// escollerPorto devolve o primeiro porto de candidatos no que se pode escoitar agora
// mesmo. Hai que chamala co servizo de File Browser xa detido, ou atoparía o porto
// ocupado polo propio File Browser dunha instalación anterior.
func escollerPorto(candidatos []int) (int, error) {
	for _, p := range candidatos {
		l, err := net.Listen("tcp", ":"+strconv.Itoa(p))
		if err == nil {
			l.Close()
			return p, nil
		}
	}
	return 0, fmt.Errorf("os portos %v están ocupados por outro programa", candidatos)
}

// baseLocalFB é a URL pola que o propio instalador fala con Quantum.
func baseLocalFB(porto int) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", porto, fbBaseURL)
}

// urlPublicaFB é a URL que ve o alumnado para un host dado (sen ":80").
func urlPublicaFB(host string, porto int) string {
	if porto == 80 {
		return "http://" + host + fbBaseURL
	}
	return fmt.Sprintf("http://%s:%d%s", host, porto, fbBaseURL)
}

// urlsAlumnado propón as URLs coas que o alumnado (Windows ou Linux) chega a este
// servidor: o nome .local (mDNS, o que deduce Tao do nome do equipo), o nome tal cal
// (resolución de Windows: LLMNR/NetBIOS, que os equipos Linux adoitan non ter) e as
// IP, que funcionan sempre.
func urlsAlumnado(porto int) []string {
	urls := []string{}
	if h, err := os.Hostname(); err == nil && h != "" {
		h = strings.ToLower(h)
		if !strings.Contains(h, ".") {
			urls = append(urls, urlPublicaFB(h+".local", porto))
		}
		urls = append(urls, urlPublicaFB(h, porto))
	}
	for _, ip := range ipsLocais() {
		urls = append(urls, urlPublicaFB(ip, porto))
	}
	return urls
}

// ipsLocais devolve as IPv4 deste equipo nas interfaces activas, sen loopback nin
// enderezos automáticos 169.254.x.x (sen DHCP: non serven para a aula).
func ipsLocais() []string {
	ips := []string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || ip4.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ip4.String())
		}
	}
	return ips
}

var clienteFB = &http.Client{Timeout: 5 * time.Second}

// agardarSaudeFB agarda a que Quantum responda 200 en /health (paso 10 do playbook).
func agardarSaudeFB(base string, intentos int, pausa time.Duration) error {
	var ultimo error
	for i := 0; i < intentos; i++ {
		resp, err := clienteFB.Get(base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			ultimo = fmt.Errorf("/health respondeu %d", resp.StatusCode)
		} else {
			ultimo = err
		}
		time.Sleep(pausa)
	}
	return fmt.Errorf("File Browser non responde en %s: %v", base, ultimo)
}

// fbPasswordHeader codifica o contrasinal para a cabeceira X-Password igual que Tao
// (tao/filebrowser.go): un "ñ" ou un "%" en cru darían 401.
func fbPasswordHeader(contrasinal string) string {
	return strings.ReplaceAll(url.QueryEscape(contrasinal), "+", "%20")
}

// comprobarLoginFB fai login como admin e reintenta mentres Quantum non estea listo ou
// limite os intentos (429, respectando Retry-After). Un 401/403 non se reintenta: o
// contrasinal non é o bo e insistir só bloquearía a conta.
func comprobarLoginFB(base, usuario, contrasinal string, intentos int) error {
	var ultimo error
	for i := 0; i < intentos; i++ {
		req, _ := http.NewRequest(http.MethodPost, base+"/api/auth/login?username="+url.QueryEscape(usuario), nil)
		req.Header.Set("X-Password", fbPasswordHeader(contrasinal))
		resp, err := clienteFB.Do(req)
		if err != nil {
			ultimo = err
			time.Sleep(2 * time.Second)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusOK:
			return nil
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return fmt.Errorf("o admin non entra (HTTP %d): o contrasinal gardado non coincide", resp.StatusCode)
		case resp.StatusCode == http.StatusTooManyRequests:
			espera := 5 * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s < 120 {
				espera = time.Duration(s) * time.Second
			}
			ultimo = fmt.Errorf("demasiados intentos (429)")
			time.Sleep(espera)
		default:
			ultimo = fmt.Errorf("login respondeu HTTP %d", resp.StatusCode)
			time.Sleep(2 * time.Second)
		}
	}
	return ultimo
}

// actualizarAdminTao pon o usuario e o contrasinal de admin de File Browser en
// <dirConfig>/dixitalizacion/admin.json, onde Tao garda as credenciais coas que xestiona
// as contas do alumnado. Mesmo motivo que en Piztu (installer/install.go): reinstalar
// File Browser recrea o admin, e un admin.json co contrasinal vello daría 401 en bucle
// ata que Quantum bloquease a conta. Só toca fbAdminUser e fbAdminPass; se o ficheiro
// non existe (Tao nunca se configurou) non fai nada e devolve false.
func actualizarAdminTao(dirConfig, usuario, contrasinal string) (bool, error) {
	ruta := filepath.Join(dirConfig, "dixitalizacion", "admin.json")
	datos, err := os.ReadFile(ruta)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(datos, &cfg); err != nil {
		return false, fmt.Errorf("%s non é un JSON válido, non se toca: %w", ruta, err)
	}
	u, _ := json.Marshal(usuario)
	c, _ := json.Marshal(contrasinal)
	cfg["fbAdminUser"], cfg["fbAdminPass"] = u, c
	novo, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(ruta, novo, 0o600)
}

// escribirServidorTao deixa ao carón de tao.exe o tao-servidor.conf (tao/servidor.go:
// rutasServidorConf), para que Tao arrinque xa co servidor desta instalación escrito.
// No equipo do profesorado o servidor é el mesmo: localhost.
func escribirServidorTao(dirTao string, porto int) error {
	if _, err := os.Stat(dirTao); err != nil {
		return nil // Tao aínda non instalado: escríbeo InstalarTao
	}
	contido := "# Xerado polo instalador de aula de Piztu: servidor File Browser deste equipo.\n" +
		urlPublicaFB("localhost", porto) + "\n"
	ruta := filepath.Join(dirTao, "tao-servidor.conf")
	// Borrar antes de escribir: en macOS pode deixalo root (o paso de File Browser) dentro
	// dun Tao.app do usuario, e o usuario pode borralo pero non sobrescribilo.
	os.Remove(ruta)
	return os.WriteFile(ruta, []byte(contido), 0o644)
}
