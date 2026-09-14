package crawler

const stealthJS = `(() => {
  Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
  window.chrome = { runtime: {}, loadTimes: function () {}, csi: function () {} };
  Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en', 'tr-TR'] });
  Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
  Object.defineProperty(navigator, 'platform', { get: () => 'MacIntel' });
})();`

const extractJS = `(() => {
  const title = document.title || '';
  const desc = (document.querySelector('meta[name="description"]') || {}).content || '';
  const canonical = (document.querySelector('link[rel="canonical"]') || {}).href || location.href;
  const jsonld = Array.from(document.querySelectorAll('script[type="application/ld+json"]'))
    .map((el) => (el.textContent || '').trim())
    .filter(Boolean);
  const links = Array.from(document.querySelectorAll('a[href]'))
    .map((a) => a.href)
    .filter(Boolean);
  const root = document.body ? document.body.cloneNode(true) : document.documentElement.cloneNode(true);
  root.querySelectorAll('script, style, noscript, svg, iframe, canvas').forEach((n) => n.remove());
  const text = (root.innerText || '').replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim();
  return { title, description: desc, canonical, text, links, jsonld, href: location.href };
})()`

const dismissConsentJS = `(() => {
  const labels = ['accept', 'accept all', 'agree', 'kabul', 'kabul et', 'izin ver', 'got it', 'tamam'];
  const nodes = Array.from(document.querySelectorAll('button, [role="button"], a'));
  for (const el of nodes) {
    const t = (el.innerText || el.getAttribute('aria-label') || '').toLowerCase().trim();
    if (!t || t.length > 40) continue;
    if (labels.some((l) => t === l || t.includes(l))) {
      try { el.click(); } catch (e) {}
    }
  }
})()`
