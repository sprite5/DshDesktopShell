(function () {
  var $ = function (id) { return document.getElementById(id); };
  var statusEl = $('status');
  var urlInput = $('url');
  var dshStatusEl = $('dsh-status');
  var dshInstallEl = $('dsh-install');
  var dshActionsEl = $('dsh-actions');
  var startBtn = $('start-managed');
  var restartBtn = $('restart-managed');
  var dshLogsEl = $('dsh-logs');
  var pollTimer = null;

  function setStatus(text, ok) {
    statusEl.hidden = false;
    statusEl.textContent = text;
    statusEl.className = 'status' + (ok ? ' ok' : ' err');
  }

  async function postJSON(path, body) {
    var res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body || {})
    });
    var data = await res.json().catch(function () { return {}; });
    return { status: res.status, data: data };
  }

  function renderDsh(state) {
    var s = state.dshState || 'idle';
    if (!state.dshAvailable) {
      var why = state.dshError ? '（' + state.dshError + '）' : '';
      dshStatusEl.textContent = '未检测到 dsh' + why;
      dshStatusEl.className = 'status-line warn';
      dshInstallEl.hidden = false;
      dshActionsEl.hidden = true;
      return;
    }
    dshInstallEl.hidden = true;
    if (state.dshRunning) {
      dshStatusEl.textContent = 'dsh 运行中 · ' + state.url;
      dshStatusEl.className = 'status-line ok';
      startBtn.hidden = true;
      restartBtn.hidden = false;
      dshActionsEl.hidden = false;
    } else if (s === 'starting') {
      dshStatusEl.textContent = 'dsh 正在启动…';
      dshStatusEl.className = 'status-line';
      startBtn.hidden = true;
      restartBtn.hidden = true;
      dshActionsEl.hidden = false;
    } else {
      var via = state.dshSource === 'npx' ? '（npx）' : '';
      dshStatusEl.textContent = '已检测到 dsh' + via + '，尚未运行';
      dshStatusEl.className = 'status-line';
      startBtn.hidden = false;
      restartBtn.hidden = true;
      dshActionsEl.hidden = false;
    }
    if (state.dshLogs && state.dshLogs.length) {
      dshLogsEl.textContent = state.dshLogs.slice(-8).join('\n');
      dshLogsEl.hidden = false;
    } else {
      dshLogsEl.hidden = true;
    }
  }

  function renderRecents(list) {
    var box = $('recent');
    box.textContent = '';
    (list || []).forEach(function (u) {
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'chip recent-chip';
      b.textContent = u;
      b.title = '连接 ' + u;
      b.addEventListener('click', function () { urlInput.value = u; connect(u); });
      box.appendChild(b);
    });
    $('recent-section').hidden = !(list && list.length);
  }

  async function connect(url) {
    var target = (url || urlInput.value || '').trim();
    if (!target) { setStatus('请输入 DSH 地址', false); return; }
    setStatus('正在连接 ' + target + ' …');
    var r = await postJSON('/__api/connect', { url: target });
    if (r.status === 200 && r.data.ok) {
      setStatus('已连接 ' + r.data.url + '，正在打开…', true);
    } else {
      setStatus('连接失败: ' + (r.data.error || ('HTTP ' + r.status)), false);
    }
  }

  async function probe() {
    var target = urlInput.value.trim();
    if (!target) { setStatus('请输入要测试的地址', false); return; }
    setStatus('正在测试 ' + target + ' …');
    var r = await postJSON('/__api/probe', { url: target });
    if (r.data.ok) {
      setStatus('可达（HTTP ' + r.data.status + '）', true);
    } else {
      setStatus('不可达: ' + (r.data.error || '未知错误'), false);
    }
  }

  function startManaged() {
    setStatus('正在启动本地 dsh…');
    postJSON('/__api/start-managed').then(function (r) {
      if (!(r.status === 200 && r.data.ok)) {
        setStatus('启动失败: ' + (r.data.error || ('HTTP ' + r.status)), false);
      }
      pollState(2500);
    });
  }

  function pollState(delay) {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
    pollTimer = setInterval(function () {
      fetch('/__api/state').then(function (res) { return res.json(); }).then(function (st) {
        renderDsh(st);
        if (st.dshRunning || !st.dshAvailable || (st.dshState !== 'starting' && st.dshState !== 'idle')) {
          if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
        }
      }).catch(function () {});
    }, delay || 2000);
  }

  async function init() {
    try {
      var res = await fetch('/__api/state');
      var state = await res.json();
      if (state.url) { urlInput.value = state.url; }
      if (state.version) { $('version').textContent = 'v' + state.version; }
      renderDsh(state);
      renderRecents(state.recent);
    } catch (e) {
      setStatus('无法读取本地状态: ' + String(e), false);
    }
  }

  $('connect').addEventListener('click', function () { connect(); });
  urlInput.addEventListener('keydown', function (e) { if (e.key === 'Enter') connect(); });
  $('probe').addEventListener('click', probe);
  startBtn.addEventListener('click', startManaged);
  restartBtn.addEventListener('click', function () { setStatus('正在重启 dsh…'); postJSON('/__api/start-managed'); pollState(2500); });
  document.querySelectorAll('.chip[data-url]').forEach(function (b) {
    b.addEventListener('click', function () { urlInput.value = b.dataset.url; });
  });

  init();
})();
