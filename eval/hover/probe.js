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
window.hovered = (svgIndex) => {
  const svg = document.querySelectorAll('svg')[svgIndex];
  const pick = (sel) => [...svg.querySelectorAll(sel)];
  const lit = pick('path.w, path.route').filter((p) => p.matches(':hover') || (getComputedStyle(p).opacity > 0.5 && p.classList.contains('route')));
  const faded = pick('path.w').filter((p) => +getComputedStyle(p).opacity < 0.5).length;
  const boxes = pick('.box, .port').filter((b) => +getComputedStyle(b).opacity > 0.9).map((b) => b.dataset.id);
  return {
    lit: lit.map((p) => ({ cls: p.getAttribute('class'), d: p.getAttribute('d'), stroke: getComputedStyle(p).stroke, width: getComputedStyle(p).strokeWidth, title: p.querySelector('title').textContent.split('\n')[0] })),
    fadedInk: faded,
    brightShapes: boxes,
  };
};
window.shown = (svgIndex) => {
  const svg = document.querySelectorAll('svg')[svgIndex];
  const hov = [...svg.querySelectorAll('.box, .port, path.w, path.route')].filter((e) => e.matches(':hover')).map((e) => (e.dataset.id || e.getAttribute('class')));
  const vis = [...svg.querySelectorAll('path.w, path.route')].filter((p) => +getComputedStyle(p).opacity > 0.5);
  const colours = {};
  for (const p of vis) {
    const c = getComputedStyle(p).stroke;
    colours[c] = (colours[c] || 0) + 1;
  }
  return {
    hovered: hov,
    shownStrokes: vis.length,
    of: svg.querySelectorAll('path.w').length + ' lines, ' + svg.querySelectorAll('path.route').length + ' routes',
    colours,
    shown: vis.slice(0, 12).map((p) => p.getAttribute('class').split(' ').slice(0, 2).join(' ') + ' ' + getComputedStyle(p).stroke + ' ' + p.getAttribute('d').slice(0, 60)),
  };
};
window.pick = (svgIndex) => {
  const svg = document.querySelectorAll('svg')[svgIndex];
  svg.scrollIntoView();
  const ws = [...svg.querySelectorAll('path.w')];
  const trunk = ws.filter((w) => w.dataset.net && w.getAttribute('d').split('M').length > 3)[0];
  const t = trunk.getAttribute('d').trim().split(/\s+/);
  const a = [+t[0].slice(1), +t[1]], b = [+t[2].slice(1), +t[3]];
  const stub = ws.filter((w) => w.classList.contains('joins') && w.getAttribute('d').split('M').length === 2)[0];
  const s = stub.getAttribute('d').trim().split(/\s+/);
  const boxes = [...svg.querySelectorAll('.box')].map((bx) => {
    const n = [...bx.classList].find((c) => /^p\d+$/.test(c));
    const lines = ws.filter((w) => w.classList.contains('e' + n.slice(1))).length;
    const r = bx.querySelector('rect');
    return { id: bx.dataset.id, lines, x: +r.getAttribute('x') + 30, y: +r.getAttribute('y') + 12 };
  }).sort((p, q) => q.lines - p.lines);
  return {
    trunk: [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2, trunk.querySelector('title').textContent.split('\n')[0]],
    stub: [(+s[0].slice(1) + +s[2].slice(1)) / 2, (+s[1] + +s[3]) / 2, stub.querySelector('title').textContent.split('\n')[0]],
    box: boxes[0],
  };
};
