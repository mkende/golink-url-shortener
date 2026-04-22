// GoLink UI — static JavaScript bundle.
//
// Why we self-host instead of using CDNs:
//   • A strict Content-Security-Policy (script-src 'self') blocks all
//     third-party script origins; referencing unpkg / jsDelivr would require
//     relaxing that policy and re-opening the door to supply-chain attacks.
//   • External CDN requests leak the user's IP and timing data to third parties
//     on every page load.
//   • CDN availability is out of our control; self-hosting makes the service
//     fully self-contained and air-gap friendly.
//   • Pinned version numbers in file names give the same immutable guarantee
//     as SRI hashes without the added complexity.
//
// Bundled vendor assets in this directory:
//   htmx-1.9.10.min.js   — https://github.com/bigskysoftware/htmx
//   bulma-0.9.4.min.css  — https://github.com/jgthms/bulma

document.addEventListener('DOMContentLoaded', function () {
  // Navbar burger toggle (Bulma).
  document.querySelectorAll('.navbar-burger').forEach(function (burger) {
    burger.addEventListener('click', function () {
      var target = document.getElementById(burger.dataset.target);
      burger.classList.toggle('is-active');
      burger.setAttribute('aria-expanded', burger.classList.contains('is-active'));
      if (target) { target.classList.toggle('is-active'); }
    });
  });

  // Render UTC <time class="local-time"> elements in the browser's local timezone.
  document.querySelectorAll('time.local-time').forEach(function (el) {
    var d = new Date(el.getAttribute('datetime'));
    if (isNaN(d.getTime())) { return; }
    el.textContent = d.toLocaleString(undefined, {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit'
    });
    el.title = el.getAttribute('datetime');
  });

  // Confirm dialogs: buttons with data-confirm="..." show a confirmation
  // dialog before the click proceeds; the event is cancelled on dismiss.
  document.querySelectorAll('[data-confirm]').forEach(function (el) {
    el.addEventListener('click', function (e) {
      if (!confirm(el.dataset.confirm)) {
        e.preventDefault();
      }
    });
  });

  // details.html: wire up the link-type selector.
  var linkTypeSelect = document.getElementById('link-type-select');
  if (linkTypeSelect) {
    linkTypeSelect.addEventListener('change', updateLinkTypeFields);
  }

  // new.html: wire up link-type radios and set the initial placeholder.
  var linkTypeRadios = document.querySelectorAll('input[name="link_type"]');
  if (linkTypeRadios.length > 0) {
    linkTypeRadios.forEach(function (r) {
      r.addEventListener('change', updatePlaceholder);
    });
    updatePlaceholder();
  }

  // index.html / new.html: disable Random while the name field has user-entered
  // text; re-enable after Random generates a name or the field is cleared.
  initRandomBtns();

  // index.html: "Advanced options" button navigates to /new with form values.
  var advancedBtn = document.querySelector('[data-go-advanced]');
  if (advancedBtn) {
    advancedBtn.addEventListener('click', function () {
      goToAdvanced(advancedBtn.closest('form'));
    });
  }
});

// details.html: toggle target vs alias-target fields based on the selected type.
function updateLinkTypeFields() {
  var t = document.getElementById('link-type-select').value;
  document.getElementById('target-field').style.display = (t !== 'alias') ? '' : 'none';
  document.getElementById('alias-target-field').style.display = (t === 'alias') ? '' : 'none';
}

// new.html: update the target field placeholder and label for the selected type.
var _placeholders = {
  simple:   {label: 'Target URL',               placeholder: 'https://example.com'},
  advanced: {label: 'Target URL (Go Template)', placeholder: 'https://example.com/{{ .path }}'},
  alias:    {label: 'Target Link Name',         placeholder: 'existing-link-name'}
};
function updatePlaceholder() {
  var selected = document.querySelector('input[name="link_type"]:checked');
  if (!selected) { return; }
  var info = _placeholders[selected.value] || _placeholders.simple;
  var targetInput = document.getElementById('target');
  var targetLabel = document.getElementById('target-label');
  if (targetInput) { targetInput.placeholder = info.placeholder; }
  if (targetLabel) { targetLabel.textContent = info.label; }
}

// index.html / new.html: disable Random while the name field has user-entered
// text; re-enable it after Random generates a name or the field is cleared.
// HTMX's outerHTML swap replaces the input element, so we rebind on each
// afterRequest (fired on the triggering button after the swap completes).
function initRandomBtns() {
  document.querySelectorAll('[data-random-for]').forEach(function (btn) {
    var form = btn.closest('form');
    if (!form) { return; }
    bindRandomInput(btn, form);
    btn.addEventListener('htmx:afterRequest', function () {
      bindRandomInput(btn, form);
      btn.disabled = false;
    });
  });
}

function bindRandomInput(btn, form) {
  var input = form.querySelector('input[name="name"]');
  if (!input) { return; }
  function update() { btn.disabled = !!input.value.trim(); }
  update();
  input.addEventListener('input', update);
}

// index.html: navigate to /new, carrying over name and target from the form.
function goToAdvanced(form) {
  var params = new URLSearchParams();
  var name = form.elements['name'].value;
  var target = form.elements['target'].value;
  if (name) { params.set('name', name); }
  if (target) { params.set('target', target); }
  var qs = params.toString();
  window.location.href = '/new' + (qs ? '?' + qs : '');
}
