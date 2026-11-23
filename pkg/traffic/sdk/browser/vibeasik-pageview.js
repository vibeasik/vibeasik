/* vibeasik Traffic Analyzer - pelacak kunjungan halaman (tanpa cookie, tanpa storage, tanpa kunci di browser).
 * Pasang:  <script src="/vibeasik-pageview.js" data-endpoint="/_vibeasik/pv" defer></script>
 * Server aplikasi melayani endpoint itu lewat PageviewHandler() dari SDK (VIBEASIK_TRAFFIC_URL/KEY ada di server).
 * Yang dikirim: path halaman (tanpa query/hash) dan document.referrer. Dikirim saat halaman dimuat dan saat path
 * berubah (pushState/replaceState/popstate pada aplikasi satu halaman). Dilewati bila Do Not Track,
 * Global Privacy Control, atau browser otomatis (webdriver) aktif.
 */
(function () {
  'use strict'
  var w = window, n = navigator, d = document
  if (w.__vibeasikPageview) return
  w.__vibeasikPageview = true
  if (n.doNotTrack === '1' || w.doNotTrack === '1' || n.globalPrivacyControl === true || n.webdriver) return

  var script = d.currentScript
  var endpoint = (script && script.getAttribute('data-endpoint')) || '/_vibeasik/pv'
  var last = null
  var ref = d.referrer || ''

  function send(path, referrer) {
    var body = JSON.stringify({ p: path, r: referrer })
    try {
      if (n.sendBeacon && n.sendBeacon(endpoint, new Blob([body], { type: 'text/plain' }))) return
    } catch (e) {}
    try {
      w.fetch(endpoint, { method: 'POST', body: body, keepalive: true, credentials: 'omit', headers: { 'Content-Type': 'text/plain' } })['catch'](function () {})
    } catch (e) {}
  }

  function track() {
    var path = w.location.pathname
    if (path === last) return
    last = path
    send(path, ref)
    ref = w.location.origin + path // perujuk navigasi berikutnya = halaman ini (hanya host yang dipakai server)
  }

  function wrap(name) {
    var orig = w.history[name]
    if (typeof orig !== 'function') return
    w.history[name] = function () {
      var r = orig.apply(this, arguments)
      track()
      return r
    }
  }

  wrap('pushState')
  wrap('replaceState')
  w.addEventListener('popstate', track)
  track()
})()
