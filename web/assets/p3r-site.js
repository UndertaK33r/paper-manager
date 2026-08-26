/* ============================================================================
   p3r-site.js — the motion layer for the "Atlus 官网风" editorial mode.
   Part of the `p3r-aesthetic` skill. Vanilla JS, zero dependencies.

   Provides:
   1. Cover-flow hero: cycles .p3r-site-hero__slide--active every 4s and keeps
      dots + arrows in sync (official site effect: active slide scale(1.22),
      siblings dimmed).
   2. Scroll reveal: .p3r-reveal elements fade/slide in once on first view
      (IntersectionObserver), so even a fully static page gains motion.

   All animations respect prefers-reduced-motion (then only a plain swap of
   the active slide stays, without transitions).
   Include after p3r-site.css: <script defer src="p3r-site.js"></script>
   ========================================================================== */
(function () {
  'use strict';
  var reduced = typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ── 1. cover-flow hero ───────────────────────────────────────────────────
  var track = document.querySelector('.p3r-site-hero__track');
  if (track) {
    var slides = Array.prototype.slice.call(track.querySelectorAll('.p3r-site-hero__slide'));
    var dots = Array.prototype.slice.call(document.querySelectorAll('.p3r-site-hero__dot'));
    var current = slides.findIndex(function (s) { return s.classList.contains('p3r-site-hero__slide--active'); });
    if (current < 0) current = 0;
    var timer = null;

    var apply = function (index) {
      slides.forEach(function (s, i) { s.classList.toggle('p3r-site-hero__slide--active', i === index); });
      dots.forEach(function (d, i) { d.classList.toggle('p3r-site-hero__dot--active', i === index); });
    };
    var show = function (index) {
      current = (index + slides.length) % slides.length;
      apply(current);
    };
    var start = function () {
      if (reduced || slides.length < 2) return;
      timer = setInterval(function () { show(current + 1); }, 4000);
    };
    apply(current);
    start();

    var arrowsPrev = Array.prototype.slice.call(document.querySelectorAll('.p3r-site-hero__arrow--prev'));
    var arrowsNext = Array.prototype.slice.call(document.querySelectorAll('.p3r-site-hero__arrow--next'));
    arrowsPrev.forEach(function (el) {
      el.addEventListener('click', function () { show(current - 1); });
    });
    arrowsNext.forEach(function (el) {
      el.addEventListener('click', function () { show(current + 1); });
    });
    dots.forEach(function (d, i) {
      d.addEventListener('click', function () { show(i); });
      if (!reduced && slides.length >= 2) d.setAttribute('role', 'button');
    });
  }

  // ── 2. scroll reveal ─────────────────────────────────────────────────────
  var reveals = Array.prototype.slice.call(document.querySelectorAll('.p3r-reveal'));
  if (reveals.length > 0) {
    var onAll = function () {
      reveals.forEach(function (el) { el.classList.add('p3r-reveal--on'); });
    };
    if (reduced || typeof IntersectionObserver !== 'function') {
      onAll();
    } else {
      var io = new IntersectionObserver(function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            entry.target.classList.add('p3r-reveal--on');
            io.unobserve(entry.target);
          }
        });
      }, { threshold: 0.12 });
      reveals.forEach(function (el) { io.observe(el); });
    }
  }
})();
