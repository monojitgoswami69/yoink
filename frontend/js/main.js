(function () {
  'use strict';

  var header = document.querySelector('.site-header');
  var burger = document.getElementById('nav-burger');
  var prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var progressBar = document.querySelector('#docs-progress span');
  var toTop = document.getElementById('to-top');

  function onScroll() {
    if (header) header.classList.toggle('scrolled', window.scrollY > 8);
    if (progressBar) {
      var max = document.documentElement.scrollHeight - window.innerHeight;
      progressBar.style.width = max > 0 ? Math.min(100, (window.scrollY / max) * 100) + '%' : '0%';
    }
    if (toTop) toTop.classList.toggle('show', window.scrollY > 600);
  }
  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();

  if (toTop) {
    toTop.addEventListener('click', function () {
      window.scrollTo({ top: 0, behavior: prefersReduced ? 'auto' : 'smooth' });
    });
  }

  if (burger) {
    var links = document.querySelectorAll('#nav-links a');
    var closeMenu = function () {
      document.body.classList.remove('menu-open');
      burger.setAttribute('aria-expanded', 'false');
    };
    burger.addEventListener('click', function () {
      var open = document.body.classList.toggle('menu-open');
      burger.setAttribute('aria-expanded', String(open));
    });
    links.forEach(function (link) {
      link.addEventListener('click', closeMenu);
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && document.body.classList.contains('menu-open')) {
        closeMenu();
        burger.focus();
      }
    });
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); } catch (err) { /* noop */ }
    document.body.removeChild(ta);
    return Promise.resolve();
  }

  document.querySelectorAll('.copy-btn').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var holder = btn.closest('.codeblock, .install-bar');
      var code = holder ? holder.querySelector('code') : null;
      if (!code || !code.textContent) return;
      copyText(code.textContent).catch(function () {});
      var label = btn.querySelector('.copy-label');
      btn.classList.add('copied');
      if (label) {
        label.textContent = 'Copied ✓';
        window.setTimeout(function () {
          btn.classList.remove('copied');
          label.textContent = 'Copy';
        }, 1600);
      }
    });
  });

  document.querySelectorAll('[role="tablist"]').forEach(function (list) {
    var tabs = Array.prototype.slice.call(list.querySelectorAll('[role="tab"]'));
    function activate(tab, focus) {
      tabs.forEach(function (t) {
        var selected = t === tab;
        t.setAttribute('aria-selected', String(selected));
        t.tabIndex = selected ? 0 : -1;
        var panel = document.getElementById(t.getAttribute('aria-controls'));
        if (panel) panel.hidden = !selected;
      });
      if (focus) tab.focus();
    }
    tabs.forEach(function (tab, i) {
      tab.addEventListener('click', function () { activate(tab, false); });
      tab.addEventListener('keydown', function (e) {
        var next = null;
        if (e.key === 'ArrowRight') next = tabs[(i + 1) % tabs.length];
        else if (e.key === 'ArrowLeft') next = tabs[(i - 1 + tabs.length) % tabs.length];
        else if (e.key === 'Home') next = tabs[0];
        else if (e.key === 'End') next = tabs[tabs.length - 1];
        if (next) {
          e.preventDefault();
          activate(next, true);
        }
      });
    });
  });

  var search = document.getElementById('cmd-search');
  if (search) {
    var rows = document.querySelectorAll('.cmd-row');
    var groups = document.querySelectorAll('.cmd-group');
    var empty = document.getElementById('cmd-empty');
    search.addEventListener('input', function () {
      var q = search.value.trim().toLowerCase();
      var any = false;
      rows.forEach(function (row) {
        var hit = !q || row.textContent.toLowerCase().indexOf(q) !== -1;
        row.hidden = !hit;
        if (hit) any = true;
      });
      groups.forEach(function (group) {
        group.hidden = !group.querySelector('.cmd-row:not([hidden])');
      });
      if (empty) empty.hidden = any;
    });
  }

  var termBody = document.getElementById('t-body');
  var replayBtn = document.getElementById('t-replay');

  if (termBody) {
    var lines = Array.prototype.slice.call(termBody.querySelectorAll('.t-line'));
    var cmdText = termBody.querySelector('.t-cmd-text');
    var cmdFull = cmdText ? cmdText.textContent : '';
    var timers = [];

    function clearTimers() {
      timers.forEach(clearTimeout);
      timers = [];
    }

    function showAll() {
      clearTimers();
      termBody.classList.add('done');
      lines.forEach(function (line) { line.classList.add('on'); });
      if (cmdText) cmdText.textContent = cmdFull;
    }

    function play() {
      clearTimers();
      termBody.classList.remove('done');
      lines.forEach(function (line) { line.classList.remove('on'); });
      if (!cmdText || !cmdFull) { showAll(); return; }
      cmdText.textContent = '';
      var i = 0;
      function type() {
        i += 1;
        cmdText.textContent = cmdFull.slice(0, i);
        if (i < cmdFull.length) {
          timers.push(window.setTimeout(type, 16));
        } else {
          var delay = 300;
          lines.forEach(function (line) {
            if (line.dataset.type === 'cmd') return;
            timers.push(window.setTimeout(function () {
              line.classList.add('on');
            }, delay));
            delay += line.dataset.type === 'step' ? 240 : 160;
          });
          timers.push(window.setTimeout(function () {
            termBody.classList.add('done');
          }, delay + 250));
        }
      }
      timers.push(window.setTimeout(type, 350));
    }

    if (prefersReduced || !('IntersectionObserver' in window)) {
      showAll();
      if (replayBtn) replayBtn.hidden = true;
    } else {
      termBody.classList.add('t-anim');
      var played = false;
      var io = new IntersectionObserver(function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting && !played) {
            played = true;
            io.disconnect();
            play();
          }
        });
      }, { threshold: 0.35 });
      io.observe(termBody);
      if (replayBtn) replayBtn.addEventListener('click', play);
    }
  }

  var docsNav = document.querySelector('.docs-nav');
  if (docsNav && 'IntersectionObserver' in window) {
    var navLinks = Array.prototype.slice.call(docsNav.querySelectorAll('.doc-link'));
    var byId = {};
    navLinks.forEach(function (link) {
      var id = (link.getAttribute('href') || '').slice(1);
      if (id) byId[id] = link;
    });
    var spy = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        navLinks.forEach(function (l) { l.classList.remove('active'); });
        var link = byId[entry.target.id];
        if (link) link.classList.add('active');
      });
    }, { rootMargin: '-30% 0px -60% 0px' });
    document.querySelectorAll('.doc-block[id]').forEach(function (section) {
      spy.observe(section);
    });
  }

  var year = document.getElementById('year');
  if (year) year.textContent = String(new Date().getFullYear());
})();
