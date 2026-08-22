(function () {
  'use strict';

  var header = document.querySelector('.site-header');
  var burger = document.getElementById('nav-burger');
  var prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  function onScroll() {
    if (!header) return;
    header.classList.toggle('scrolled', window.scrollY > 8);
  }
  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();

  if (burger) {
    burger.addEventListener('click', function () {
      var open = document.body.classList.toggle('menu-open');
      burger.setAttribute('aria-expanded', String(open));
    });
    document.querySelectorAll('#nav-links a').forEach(function (link) {
      link.addEventListener('click', function () {
        document.body.classList.remove('menu-open');
        burger.setAttribute('aria-expanded', 'false');
      });
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && document.body.classList.contains('menu-open')) {
        document.body.classList.remove('menu-open');
        burger.setAttribute('aria-expanded', 'false');
        burger.focus();
      }
    });
  }

  function fallbackCopy(text) {
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand('copy');
    } catch (err) { /* noop */ }
    document.body.removeChild(ta);
  }

  document.querySelectorAll('.copy-btn').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var text = btn.getAttribute('data-copy-text');
      if (!text) {
        var target = btn.getAttribute('data-copy-target');
        var el = target ? document.querySelector(target) : null;
        if (!el) {
          var block = btn.closest('.copyable');
          el = block ? block.querySelector('code') : null;
        }
        text = el ? el.textContent : '';
      }
      if (!text) return;
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).catch(function () {
          fallbackCopy(text);
        });
      } else {
        fallbackCopy(text);
      }
      var label = btn.querySelector('.copy-label');
      btn.classList.add('copied');
      if (label) {
        label.dataset.original = label.textContent;
        label.textContent = 'Copied ✓';
      }
      window.setTimeout(function () {
        btn.classList.remove('copied');
        if (label && label.dataset.original) {
          label.textContent = label.dataset.original;
        }
      }, 1600);
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
    var rows = Array.prototype.slice.call(document.querySelectorAll('.cmd-row'));
    var groups = Array.prototype.slice.call(document.querySelectorAll('.cmd-group'));
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
        var visible = group.querySelector('.cmd-row:not([hidden])');
        group.hidden = !visible;
      });
      if (empty) empty.hidden = any;
    });
  }

  var termBody = document.getElementById('t-body');
  var replayBtn = document.getElementById('t-replay');

  if (termBody) {
    var lines = Array.prototype.slice.call(termBody.querySelectorAll('.t-line'));
    var timers = [];

    function clearTimers() {
      timers.forEach(clearTimeout);
      timers = [];
    }

    function showAll() {
      clearTimers();
      termBody.classList.add('done');
      lines.forEach(function (line) { line.classList.add('on'); });
      var cmd = termBody.querySelector('.t-cmd-text');
      if (cmd && cmd.dataset.text) cmd.textContent = cmd.dataset.text;
    }

    function reset() {
      clearTimers();
      termBody.classList.remove('done');
      lines.forEach(function (line) {
        line.classList.remove('on');
      });
    }

    function play() {
      reset();
      var cmdLine = termBody.querySelector('[data-type="cmd"]');
      if (!cmdLine) { showAll(); return; }
      var textEl = cmdLine.querySelector('.t-cmd-text');
      var full = textEl ? textEl.dataset.text : '';
      if (!textEl || !full) { showAll(); return; }
      textEl.textContent = '';
      var i = 0;
      function type() {
        i += 1;
        textEl.textContent = full.slice(0, i);
        if (i < full.length) {
          timers.push(window.setTimeout(type, 16));
        } else {
          revealRest();
        }
      }
      function revealRest() {
        var delay = 300;
        lines.forEach(function (line) {
          if (line === cmdLine) return;
          var step = line.dataset.type === 'step' ? 240 : 160;
          timers.push(window.setTimeout(function () {
            line.classList.add('on');
          }, delay));
          delay += step;
        });
        timers.push(window.setTimeout(function () {
          termBody.classList.add('done');
        }, delay + 250));
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
            play();
            io.disconnect();
          }
        });
      }, { threshold: 0.35 });
      io.observe(termBody);
      if (replayBtn) {
        replayBtn.addEventListener('click', play);
      }
    }
  }

  var year = document.getElementById('year');
  if (year) year.textContent = String(new Date().getFullYear());
})();
