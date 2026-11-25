/* vibeasik Teks Website - mengirim semua teks yang tampil di halaman ke vibeasik (arah: website -> vibeasik).
 * Pasang (hanya di lingkungan dev/staging):
 *   <script src="/vibeasik-texts.js" data-url="http://localhost:8765/api/texts/ingest" data-key="KUNCI" defer></script>
 * atau: window.VIBEASIK_TEXTS = { url, key } sebelum skrip ini dimuat.
 *
 * Yang dikirim: teks yang terlihat (judul, paragraf, tombol, tautan, label), placeholder/alt/aria-label, judul halaman
 * dan meta description, beserta path halaman (tanpa query/hash), nama section, dan selector pendek elemennya.
 * TIDAK dikirim: nilai input/textarea/select, isi contenteditable, cookie, header, storage, URL lengkap dengan query.
 * Dilewati: script, style, code/pre, elemen tersembunyi, dan elemen/anak elemen ber-atribut data-vibeasik-ignore.
 * Memberi nama section: data-vibeasik-section="Nama" pada elemen pembungkus (opsional; tanpa itu dipakai
 * header/footer/nav/section + judulnya).
 * Dinamis: halaman yang berubah (SPA, konten dimuat belakangan) dipantau lewat MutationObserver; teks yang sama
 * pada halaman & section yang sama dikirim sekali per muat halaman.
 */
(function () {
  'use strict'
  var script = document.currentScript
  var cfg = window.VIBEASIK_TEXTS || {}
  var URL_ = cfg.url || (script && script.getAttribute('data-url')) || ''
  var KEY = cfg.key || (script && script.getAttribute('data-key')) || ''
  if (!URL_ || !KEY || window.__vibeasikTexts) return
  window.__vibeasikTexts = true

  var BATCH = 300
  var MAX_ELEMENTS = 6000
  var SCAN_DELAY_MS = 800
  var SEND_DELAY_MS = 1200
  var SKIP_TAGS = { SCRIPT: 1, STYLE: 1, NOSCRIPT: 1, TEMPLATE: 1, TEXTAREA: 1, INPUT: 1, SELECT: 1, OPTION: 1, CODE: 1, PRE: 1, SVG: 1, CANVAS: 1, IFRAME: 1, HEAD: 1 }
  var INLINE = { A: 1, ABBR: 1, B: 1, BDI: 1, BDO: 1, BR: 1, CITE: 1, DATA: 1, DFN: 1, EM: 1, I: 1, MARK: 1, Q: 1, S: 1, SMALL: 1, SPAN: 1, STRONG: 1, SUB: 1, SUP: 1, TIME: 1, U: 1, WBR: 1 }
  var LANDMARKS = { HEADER: 'Header', FOOTER: 'Footer', NAV: 'Navigasi', MAIN: 'Konten utama', ASIDE: 'Sisi', FORM: 'Formulir', DIALOG: 'Dialog' }

  var sent = {}
  var queue = []
  var scanTimer = null
  var sendTimer = null

  function norm(s) { return String(s || '').replace(/\s+/g, ' ').trim() }
  function page() { return location.pathname || '/' }

  function hidden(el) {
    if (el.hidden || el.getAttribute('aria-hidden') === 'true') return true
    try {
      var st = window.getComputedStyle(el)
      return st.display === 'none' || st.visibility === 'hidden'
    } catch (e) { return false }
  }

  function ignored(el) {
    return !!(el.closest && el.closest('[data-vibeasik-ignore],[contenteditable=""],[contenteditable="true"]'))
  }

  function heading(el) {
    var h = el.querySelector && el.querySelector('h1,h2,h3,h4')
    return h ? norm(h.textContent).slice(0, 60) : ''
  }

  function sectionOf(el) {
    var named = el.closest && el.closest('[data-vibeasik-section]')
    if (named) return norm(named.getAttribute('data-vibeasik-section')).slice(0, 120)
    var n = el
    var idFallback = ''
    while (n && n !== document.body && n.nodeType === 1) {
      var tag = n.tagName
      if (LANDMARKS[tag]) {
        var lbl = n.getAttribute('aria-label')
        return LANDMARKS[tag] + (lbl ? ': ' + norm(lbl).slice(0, 60) : '')
      }
      if (n.getAttribute('role') === 'dialog') return 'Dialog'
      if (tag === 'SECTION' || tag === 'ARTICLE') {
        var h = heading(n)
        var id = n.id ? '#' + n.id : ''
        return h || id || n.getAttribute('aria-label') || tag.toLowerCase()
      }
      if (!idFallback && n.id) idFallback = '#' + n.id
      n = n.parentElement
    }
    return idFallback ? 'Bagian ' + idFallback : 'Halaman'
  }

  function selectorOf(el) {
    var parts = []
    var n = el
    while (n && n.nodeType === 1 && n !== document.body && parts.length < 4) {
      var s = n.tagName.toLowerCase()
      if (n.id) { parts.unshift(s + '#' + n.id); break }
      var cls = typeof n.className === 'string' ? n.className.trim().split(/\s+/)[0] : ''
      if (cls) s += '.' + cls.replace(/[^\w-]/g, '')
      var p = n.parentElement
      if (p) {
        var same = 0
        var idx = 0
        for (var i = 0; i < p.children.length; i++) {
          if (p.children[i].tagName === n.tagName) { same++; if (p.children[i] === n) idx = same }
        }
        if (same > 1) s += ':nth-of-type(' + idx + ')'
      }
      parts.unshift(s)
      n = p
    }
    return parts.join(' > ').slice(0, 200)
  }

  function add(el, tag, text) {
    text = norm(text)
    if (text.length < 2) return
    var section = sectionOf(el)
    var pg = page()
    var key = pg + '\u0000' + section + '\u0000' + text
    if (sent[key]) return
    sent[key] = 1
    queue.push({ page: pg, section: section, selector: selectorOf(el), tag: tag, text: text.slice(0, 2000) })
  }

  var visited = 0

  function hasBlockChild(el) {
    for (var c = el.firstElementChild; c; c = c.nextElementSibling) {
      if (!INLINE[c.tagName] && !SKIP_TAGS[c.tagName]) return true
    }
    return false
  }

  // Elemen yang isinya hanya deretan tautan (menu, footer) dipisah per tautan, bukan digabung jadi satu teks.
  function linksOnly(el) {
    var anchors = 0
    for (var n = el.firstChild; n; n = n.nextSibling) {
      if (n.nodeType === 3 && norm(n.nodeValue)) return false
      if (n.nodeType === 1) {
        if (n.tagName === 'A') anchors++
        else if (norm(n.textContent)) return false
      }
    }
    return anchors > 1
  }

  function walk(el) {
    if (++visited > MAX_ELEMENTS) return
    if (el.hasAttribute && el.hasAttribute('data-vibeasik-ignore')) return
    if (hidden(el)) return
    var tag = el.tagName.toLowerCase()
    var ph = el.getAttribute('placeholder')
    if (ph) add(el, '@placeholder', ph)
    if (SKIP_TAGS[el.tagName]) return
    if (el.tagName === 'IMG') { if (el.alt) add(el, '@alt', el.alt); return }
    var al = el.getAttribute('aria-label')
    if (al) add(el, '@aria-label', al)
    if (linksOnly(el)) {
      for (var a = el.firstElementChild; a; a = a.nextElementSibling) walk(a)
      return
    }
    if (!hasBlockChild(el)) {
      if (!(el.querySelector && el.querySelector('[data-vibeasik-ignore]'))) add(el, tag, el.textContent)
      return
    }
    var own = ''
    for (var n = el.firstChild; n; n = n.nextSibling) {
      if (n.nodeType === 3) own += ' ' + n.nodeValue
      else if (n.nodeType === 1) walk(n)
    }
    add(el, tag, own)
  }

  function scan() {
    scanTimer = null
    if (!document.body) return
    visited = 0
    try {
      if (document.title) add(document.body, 'title', document.title)
      var meta = document.querySelector('meta[name="description"]')
      if (meta && meta.content) add(document.body, 'meta-description', meta.content)
      if (!ignored(document.body)) walk(document.body)
    } catch (e) { /* pemantauan tidak boleh mengganggu halaman */ }
    if (queue.length && !sendTimer) sendTimer = setTimeout(send, SEND_DELAY_MS)
  }

  function schedule() {
    if (!scanTimer) scanTimer = setTimeout(scan, SCAN_DELAY_MS)
  }

  function send() {
    sendTimer = null
    while (queue.length) {
      var batch = queue.splice(0, BATCH)
      var body = JSON.stringify({ key: KEY, origin: location.origin, texts: batch })
      try {
        // text/plain = "simple request": tanpa preflight CORS.
        if (navigator.sendBeacon && navigator.sendBeacon(URL_, new Blob([body], { type: 'text/plain' }))) continue
        fetch(URL_, { method: 'POST', mode: 'no-cors', keepalive: true, headers: { 'Content-Type': 'text/plain' }, body: body })['catch'](function () {})
      } catch (e) { /* abaikan */ }
    }
  }

  function start() {
    schedule()
    try {
      new MutationObserver(schedule).observe(document.body, { childList: true, subtree: true, characterData: true })
    } catch (e) { /* browser lama: cukup pindai saat dimuat */ }
    ;['pushState', 'replaceState'].forEach(function (fn) {
      var orig = history[fn]
      history[fn] = function () {
        var r = orig.apply(this, arguments)
        schedule()
        return r
      }
    })
    window.addEventListener('popstate', schedule)
    window.addEventListener('load', schedule)
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start)
  else start()
})()
