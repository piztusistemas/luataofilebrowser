import './app.css';
import logo from './assets/images/logo.png';

import {
    Idioma, SetIdioma, Idiomas, Traducions, Version, Estado,
    InstalarFileBrowser, InstalarTao, InstalarLua,
    DesinstalarFileBrowser, DesinstalarTao, DesinstalarLua,
} from '../bindings/piztu/instalador_aula/app.js';
import {Application, Events} from '@wailsio/runtime';
import {confirmar} from './dialogs.js';

// ── Estado global ────────────────────────────────────────────────────────────
let I18N = {};
let IDIOMAS = [{code: 'gl', nome: 'Galego'}];
let idiomaActual = 'gl';
let versionCache = '';
let estado = {};
let current = 0;

const FB_VERSION = 'v1.5.6-stable';

const STEPS = [
    {id: 'benvida', key: 'paso.benvida'},
    {id: 'filebrowser', key: 'paso.filebrowser'},
    {id: 'tao', key: 'paso.tao'},
    {id: 'lua', key: 'paso.lua'},
    {id: 'completado', key: 'paso.completado'},
    {id: 'desinstalar', key: 'paso.desinstalar', danger: true},
];

function t(clave, fallback) {
    return I18N[clave] || fallback || clave;
}

// Substitúe %s/%d na orde en que aparecen (coma fmt.Sprintf en Go).
function fmt(cadea, ...args) {
    let i = 0;
    return cadea.replace(/%[ds]/g, () => args[i++]);
}

// Os datos que veñen do sistema (rutas, URLs) escápanse antes de ir a innerHTML.
function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, (c) => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
}

const app = document.getElementById('app');

async function refrescarEstado() {
    estado = await Estado().catch(() => ({}));
}

// ── Esqueleto (barra lateral + panel) ────────────────────────────────────────
function render() {
    app.innerHTML = `
      <div class="sidebar">
        <div class="sidebar-top">
          <img src="${logo}" class="sidebar-logo" alt="Piztu">
          <div class="sidebar-lang">
            <span>${t('nav.idioma', 'Idioma')}</span>
            <select id="lang-select">
              ${IDIOMAS.map(d => `<option value="${d.code}" ${d.code === idiomaActual ? 'selected' : ''}>${d.nome}</option>`).join('')}
            </select>
          </div>
        </div>
        <div class="sidebar-sep"></div>
        <div class="sidebar-steps">
          ${STEPS.map((s, i) => `
            ${s.danger && !STEPS[i - 1]?.danger ? '<div class="sidebar-sep"></div>' : ''}
            <button class="step-btn ${i === current ? 'activo' : ''} ${s.danger ? 'danger' : ''}" data-step="${i}">${t(s.key, s.id)}</button>
          `).join('')}
        </div>
        <div class="sidebar-sep"></div>
        <div class="sidebar-info">${esc(t('app.info', '{v}').replace('{v}', versionCache))}</div>
        <div class="sidebar-info">${t('app.licenza', 'Licenza AGPLv3')}</div>
      </div>
      <div class="main-panel">
        <div class="main-content" id="main-content"></div>
        <div class="main-footer">
          <div class="progress-row">
            <div class="progress-track"><div class="progress-fill" id="progress-fill"></div></div>
            <span class="progress-label" id="progress-label"></span>
          </div>
          <div class="nav-row">
            <button class="btn-main" id="btn-prev">${t('nav.anterior')}</button>
            <span class="spacer"></span>
            <button class="btn-main" id="btn-close">${t('nav.pechar')}</button>
            <button class="btn-main primary" id="btn-next"></button>
          </div>
        </div>
      </div>
    `;

    document.getElementById('lang-select').addEventListener('change', async (e) => {
        idiomaActual = e.target.value;
        await SetIdioma(idiomaActual);
        I18N = await Traducions();
        render();
    });
    document.querySelectorAll('.step-btn').forEach((btn) => {
        btn.addEventListener('click', () => goTo(parseInt(btn.dataset.step, 10)));
    });
    document.getElementById('btn-prev').addEventListener('click', () => goTo(current - 1));
    document.getElementById('btn-close').addEventListener('click', () => Application.Quit());

    const btnNext = document.getElementById('btn-next');
    const esUltimo = STEPS[current].id === 'completado' || STEPS[current].danger;
    btnNext.textContent = esUltimo ? t('nav.finalizar') : t('nav.seguinte');
    btnNext.addEventListener('click', () => (esUltimo ? Application.Quit() : goTo(current + 1)));
    document.getElementById('btn-prev').disabled = current === 0;

    const pasos = STEPS.filter((s) => !s.danger).length;
    const pct = (Math.min(current, pasos - 1) / (pasos - 1)) * 100;
    document.getElementById('progress-fill').style.width = pct + '%';
    document.getElementById('progress-label').textContent = STEPS[current].danger
        ? t('desinst.titulo')
        : fmt(t('nav.progreso'), current + 1, pasos);

    renderStep();
}

async function goTo(n) {
    if (n < 0 || n >= STEPS.length) return;
    current = n;
    await refrescarEstado();
    render();
}

function renderStep() {
    const el = document.getElementById('main-content');
    const paso = STEPS[current].id;
    const pasos = {
        benvida: [benvidaHTML, null],
        filebrowser: [filebrowserHTML, wireFilebrowser],
        tao: [taoHTML, wireTao],
        lua: [luaHTML, wireLua],
        completado: [completadoHTML, null],
        desinstalar: [desinstalarHTML, wireDesinstalar],
    };
    const [html, wire] = pasos[paso];
    el.innerHTML = html();
    if (wire) wire();
}

function addLogLine(box, text) {
    const div = document.createElement('div');
    div.textContent = text;
    box.appendChild(div);
    box.scrollTop = box.scrollHeight;
}

function mostrarErro(id, texto) {
    const el = document.getElementById(id);
    el.textContent = texto;
    el.style.display = texto ? 'block' : 'none';
}

// arrincarTarefa: limpa o log, bloquea o botón e chama ao binding. Se xa había outra
// tarefa en marcha (o binding devolve false), avisa e volve activar o botón.
async function arrincarTarefa(btn, logId, chamada) {
    btn.disabled = true;
    const box = document.getElementById(logId);
    box.style.display = 'block';
    box.innerHTML = '';
    const ok = await chamada();
    if (!ok) {
        addLogLine(box, t('ocupado'));
        btn.disabled = false;
    }
}

// Cada tarefa emite <prefixo>_log e <prefixo>_completo (ver app.go: tarefa).
function escoitarTarefa(prefixo, logId, btnId, aoRematar) {
    Events.On(prefixo + '_log', ({data}) => {
        const box = document.getElementById(logId);
        if (box) addLogLine(box, data);
    });
    Events.On(prefixo + '_completo', async ({data}) => {
        await refrescarEstado();
        const btn = document.getElementById(btnId);
        if (btn) btn.disabled = false;
        if (aoRematar) aoRematar(data);
    });
}

function liñaEstado(nome, ok, detalle = '') {
    return `<div class="estado-item"><b>${nome}</b>
      <span class="${ok ? 'estado-ok' : 'estado-non'}">${ok ? '✓ ' + t('estado.instalado') : t('estado.non')}</span>
      ${detalle ? `<div class="nota">${esc(detalle)}</div>` : ''}</div>`;
}

function estadoHTML() {
    return `<div class="estado-grid">
      ${liñaEstado('File Browser', estado.fileBrowser, estado.porto ? 'porto ' + estado.porto : '')}
      ${liñaEstado('Tao', estado.tao)}
      ${liñaEstado('Lúa', estado.lua)}
    </div>`;
}

function urlsHTML(urls) {
    if (!urls || !urls.length) return '';
    return `
      <div class="card">
        <h3>${t('fb.urls.titulo')}</h3>
        <ul class="urls">${urls.map((u) => `<li>${esc(u)}</li>`).join('')}</ul>
        <p class="nota">${t('fb.urls.nota')}</p>
      </div>`;
}

// ── Benvida ──────────────────────────────────────────────────────────────────
function benvidaHTML() {
    const bloques = ['fb', 'tao', 'lua', 'alumnado'];
    return `
      <h1 class="titulo">${t('benvida.titulo')}</h1>
      <p class="desc">${t('benvida.desc')}</p>
      ${bloques.map((b) => `
        <div class="card">
          <h3>${t('benvida.' + b + '.titulo')}</h3>
          <p>${t('benvida.' + b + '.corpo')}</p>
        </div>`).join('')}
      <div class="card">
        <h3>${t('benvida.estado')}</h3>
        ${estadoHTML()}
      </div>
    `;
}

// ── 1. File Browser ──────────────────────────────────────────────────────────
function filebrowserHTML() {
    return `
      <div class="card">
        <h3>${t('fb.card')}</h3>
        <p>${esc(fmt(t('fb.desc'), FB_VERSION))}</p>
        ${estado.fileBrowser ? `<div class="aviso">${t('fb.reinstalar')}</div>` : ''}
        <div class="aviso-verde">
          <h4>${t('fb.dous.titulo')}</h4>
          <p>${t('fb.dous.corpo')}</p>
          <div class="check-row">
            <input type="checkbox" id="fb-check">
            <label for="fb-check">${t('fb.dous.check')}</label>
          </div>
        </div>
        <div class="sep"></div>
        <div class="form-grid">
          <div class="form-row">
            <label>${t('fb.form.usuario')}</label>
            <input type="text" id="fb-user" autocomplete="off" value="admin">
          </div>
          <div class="form-row">
            <label>${t('fb.form.pass')}</label>
            <input type="password" id="fb-pass" autocomplete="new-password">
          </div>
          <div class="form-row">
            <label>${t('fb.form.pass2')}</label>
            <input type="password" id="fb-pass2" autocomplete="new-password">
          </div>
        </div>
        <button class="btn-main primary" id="btn-fb" disabled>${t('fb.btn')}</button>
        <p class="error-msg" id="fb-error" style="display:none;"></p>
        <div class="log-box" id="fb-log" style="display:none;"></div>
      </div>
      <div id="fb-urls">${urlsHTML(estado.urls)}</div>
    `;
}

function wireFilebrowser() {
    const btn = document.getElementById('btn-fb');
    const chk = document.getElementById('fb-check');
    // Bloqueado ata ler o aviso: "admin" (File Browser) e "director" (Tao) confúndense.
    chk.addEventListener('change', () => { btn.disabled = !chk.checked; });
    btn.addEventListener('click', async () => {
        const usuario = document.getElementById('fb-user').value.trim();
        const pass = document.getElementById('fb-pass').value;
        const pass2 = document.getElementById('fb-pass2').value;
        mostrarErro('fb-error', '');
        if (!usuario || !pass) return mostrarErro('fb-error', t('fb.faltan'));
        // Só letras, díxitos, . - _: ademais detén o erro de escribir aquí o contrasinal.
        if (!/^[A-Za-z0-9._-]+$/.test(usuario)) return mostrarErro('fb-error', t('fb.user.invalido'));
        if ([...pass].length < 12) return mostrarErro('fb-error', t('fb.pass.curto'));
        if (pass !== pass2) return mostrarErro('fb-error', t('fb.pass.distintos'));
        if (estado.fileBrowser) {
            const seguir = await confirmar(t('fb.reinstalar'), {
                aceptar: t('fb.btn'), cancelar: t('dialogo.cancelar'), perigo: true,
            });
            if (!seguir) return;
        }
        await arrincarTarefa(btn, 'fb-log', () => InstalarFileBrowser(usuario, pass));
    });
}

escoitarTarefa('fb', 'fb-log', 'btn-fb', (res) => {
    const caixa = document.getElementById('fb-urls');
    if (caixa && res?.ok) caixa.innerHTML = urlsHTML(res.urls);
    // Os contrasinais non quedan escritos no formulario despois de instalar.
    for (const id of ['fb-pass', 'fb-pass2']) {
        const el = document.getElementById(id);
        if (el && res?.ok) el.value = '';
    }
});

// ── 2. Tao ───────────────────────────────────────────────────────────────────
function taoHTML() {
    return `
      <div class="card">
        <h3>${t('tao.card')}</h3>
        <p>${esc(fmt(t('tao.desc'), estado.dirTao || ''))}</p>
        <div class="sep"></div>
        <button class="btn-main primary" id="btn-tao">${t('tao.btn')}</button>
        <div class="log-box" id="tao-log" style="display:none;"></div>
      </div>
      <div class="card">
        <h3>${t('tao.primeiro.titulo')}</h3>
        <p>${t('tao.primeiro.corpo')}</p>
      </div>
    `;
}

function wireTao() {
    const btn = document.getElementById('btn-tao');
    btn.addEventListener('click', () => arrincarTarefa(btn, 'tao-log', () => InstalarTao()));
}
escoitarTarefa('tao', 'tao-log', 'btn-tao');

// ── 3. Lúa ───────────────────────────────────────────────────────────────────
function luaHTML() {
    return `
      <div class="card">
        <h3>${t('lua.card')}</h3>
        <p>${esc(fmt(t('lua.desc'), estado.dirLua || ''))}</p>
        <div class="sep"></div>
        <label class="check-simple"><input type="checkbox" id="lua-deps" checked> ${t('lua.deps')}</label>
        <button class="btn-main primary" id="btn-lua">${t('lua.btn')}</button>
        <div class="log-box" id="lua-log" style="display:none;"></div>
      </div>
    `;
}

function wireLua() {
    const btn = document.getElementById('btn-lua');
    btn.addEventListener('click', () => {
        const deps = document.getElementById('lua-deps').checked;
        arrincarTarefa(btn, 'lua-log', () => InstalarLua(deps));
    });
}
escoitarTarefa('lua', 'lua-log', 'btn-lua');

// ── Completado ───────────────────────────────────────────────────────────────
function completadoHTML() {
    return `
      <h1 class="titulo">${t('fin.titulo')}</h1>
      <p class="desc">${t('fin.desc')}</p>
      <div class="card">${estadoHTML()}
        ${estado.fileBrowser ? `
          <p class="nota">${esc(fmt(t('fin.arquivos'), estado.dirArquivos))}</p>
          <p class="nota">${esc(fmt(t('fin.rexistro'), estado.rexistro))}</p>` : ''}
      </div>
      ${urlsHTML(estado.urls)}
      <div class="card">
        <h3>${t('fin.alumnado.titulo')}</h3>
        <p>${t('fin.alumnado.corpo')}</p>
      </div>
    `;
}

// ── Zona de perigo ───────────────────────────────────────────────────────────
// Cada tarxeta quita só a súa peza; todas piden escribir a palabra e confirmar.
const DESINST = [
    {id: 'fb', opcion: 'desinst.fb.datos', accion: DesinstalarFileBrowser},
    {id: 'tao', opcion: 'desinst.tao.datos', accion: DesinstalarTao},
    {id: 'lua', opcion: 'desinst.lua.deps', accion: DesinstalarLua},
];

function desinstalarHTML() {
    const palabra = t('desinst.palabra');
    return `
      <h1 class="titulo">${t('desinst.titulo')}</h1>
      <p class="desc">${t('desinst.intro')}</p>
      ${DESINST.map(({id, opcion}) => `
        <div class="card danger">
          <h3>${t('desinst.' + id + '.card')}</h3>
          <p>${t('desinst.' + id + '.desc')}</p>
          <div class="sep"></div>
          <label class="check-simple"><input type="checkbox" id="desinst-${id}-opcion">
            ${esc(fmt(t(opcion), estado.dirArquivos || ''))}</label>
          <div class="form-grid">
            <div class="form-row">
              <label>${esc(fmt(t('desinst.confirmar.label'), palabra))}</label>
              <input type="text" id="desinst-${id}-confirm" autocomplete="off">
            </div>
          </div>
          <button class="btn-main danger" id="btn-desinst-${id}" disabled>${t('desinst.' + id + '.btn')}</button>
          <div class="log-box" id="desinst-${id}-log" style="display:none;"></div>
        </div>`).join('')}
    `;
}

function wireDesinstalar() {
    const palabra = t('desinst.palabra').toUpperCase();
    for (const {id, accion} of DESINST) {
        const input = document.getElementById(`desinst-${id}-confirm`);
        const btn = document.getElementById(`btn-desinst-${id}`);
        const opcion = document.getElementById(`desinst-${id}-opcion`);
        input.addEventListener('input', () => {
            btn.disabled = input.value.trim().toUpperCase() !== palabra;
        });
        btn.addEventListener('click', async () => {
            const ok = await confirmar(t('desinst.' + id + '.confirm'), {
                aceptar: t('desinst.' + id + '.btn'), cancelar: t('dialogo.cancelar'), perigo: true,
            });
            if (!ok) return;
            input.value = '';
            await arrincarTarefa(btn, `desinst-${id}-log`, () => accion(opcion.checked));
        });
    }
}

for (const {id} of DESINST) {
    // Tras desinstalar non se reactiva o botón: hai que volver escribir a palabra.
    escoitarTarefa('desinst' + id, `desinst-${id}-log`, '__ningun__');
}

// ── Arranque ─────────────────────────────────────────────────────────────────
async function bootstrap() {
    [idiomaActual, I18N, IDIOMAS, versionCache] = await Promise.all([
        Idioma(), Traducions(), Idiomas(), Version().catch(() => ''),
    ]);
    await refrescarEstado();
    render();
}

bootstrap();
