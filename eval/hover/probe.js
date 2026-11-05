// Load into a blueprint page, then point a real mouse at an SVG point:
// probe(svg, x, y) parks a 2px button that takes no pointer there, and the
// devtools hover on that button moves the mouse onto the drawing beneath.
// lit(svg) reads back what the page lights while the pointer rests.
window.probe = (svgIndex, x, y) => {
  const svg = document.querySelectorAll('svg')[svgIndex];
  let el = document.getElementById('zprobe');
  if (!el) {
    el = document.createElement('button');
    el.id = 'zprobe';
    el.setAttribute('aria-label', 'zprobe');
    el.style.cssText = 'position:fixed;width:2px;height:2px;padding:0;border:0;margin:0;background:transparent;pointer-events:none;z-index:9999';
    document.body.appendChild(el);
  }
  const p = svg.createSVGPoint();
  p.x = x; p.y = y;
  const c = p.matrixTransform(svg.getScreenCTM());
  el.style.left = (c.x - 1) + 'px';
  el.style.top = (c.y - 1) + 'px';
  return [c.x, c.y];
};
window.lit = (svgIndex) => {
  const svg = document.querySelectorAll('svg')[svgIndex];
  const pick = (sel) => [...svg.querySelectorAll(sel)];
  const shown = (e) => e.getClientRects().length > 0 && getComputedStyle(e).display !== 'none';
  const under = pick('path.w, .box, .port').filter((e) => e.matches(':hover'));
  const routes = pick('g.route').filter((g) => +getComputedStyle(g).opacity > 0.5).map((g) => {
    const way = [...g.querySelectorAll('path.way')].find((p) => shown(p) && (!p.parentElement.classList.contains('lane') || getComputedStyle(p.parentElement).display !== 'none'));
    const heads = [...way.parentElement.querySelectorAll(':scope > use')].map((u) => {
      const h = document.querySelector(u.getAttribute('href') + ' path');
      return getComputedStyle(u).getPropertyValue('--c').trim() + ' ' + (h.style.fill === 'none' ? 'stroke' : 'fill');
    });
    return { line: g.getAttribute('class').split(' ').filter((c) => /^(r\d+|e\d+|bypass|mutual|back)$/.test(c)).join(' '), d: way.getAttribute('d'), stroke: getComputedStyle(way).stroke, heads };
  });
  return {
    pointed: under.map((e) => (e.dataset.id || e.getAttribute('class')) + (e.querySelector(':scope > title') ? ' | title: ' + e.querySelector(':scope > title').textContent.replace(/\n/g, ' / ') : '')),
    routes,
    inkOpacityMax: Math.max(...pick('path.w').map((p) => +getComputedStyle(p).opacity)),
    brightShapes: pick('.box, .port').filter((b) => +getComputedStyle(b).opacity > 0.9).map((b) => b.dataset.id),
    marks: pick('.lab, .count').filter((m) => +getComputedStyle(m).opacity > 0.9).map((m) => m.textContent),
  };
};
