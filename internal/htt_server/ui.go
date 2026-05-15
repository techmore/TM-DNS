package htt_server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TM-DNS</title>
  <style>
    @import url('https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&family=Inter:wght@400;500;600;700;800&display=swap');
    * { box-sizing: border-box; }
    :root {
      --olive-50:#f7f8f4; --olive-100:#eef0e6; --olive-200:#dde1d0; --olive-300:#c4c9b0; --olive-400:#a7ae8b;
      --olive-500:#8a9269; --olive-600:#6e754b; --olive-700:#575d3d; --olive-800:#464a34; --olive-900:#3b3e2d; --olive-950:#1f2117;
      --stone-50:#fafaf9; --stone-100:#f5f5f4; --stone-200:#e7e5e4; --stone-300:#d6d3d1; --stone-500:#78716c; --stone-700:#44403c; --stone-900:#1c1917;
      --red:#c0392b; --amber:#b07d2a; --green:#5a8a5e; --blue:#4e8da3;
      --bg:var(--olive-300); --surface:var(--olive-200); --surface2:var(--olive-100); --border:var(--olive-400); --text:var(--stone-900); --muted:var(--stone-500);
    }
    body { margin:0; font-family:Inter,system-ui,sans-serif; background:var(--bg); color:var(--text); font-size:14px; line-height:1.45; }
    h1,h2,h3 { font-family:"Instrument Serif",serif; margin:0; line-height:1.1; }
    .topnav { position:sticky; top:0; z-index:10; background:rgba(247,248,244,.88); backdrop-filter:blur(12px); border-bottom:1px solid rgba(221,225,208,.7); }
    .topnav-inner { max-width:1680px; height:58px; margin:0 auto; padding:0 18px; display:flex; align-items:center; gap:14px; justify-content:space-between; }
    .brand { display:flex; align-items:center; gap:10px; font-family:"Instrument Serif",serif; font-size:20px; font-weight:700; color:var(--olive-950); }
    .brand-mark { width:30px; height:30px; display:grid; place-items:center; border-radius:7px; background:var(--olive-100); color:var(--olive-950); border:1px solid var(--olive-500); font-family:Inter,sans-serif; font-weight:800; }
    .tabs { display:flex; gap:4px; flex-wrap:wrap; justify-content:flex-end; }
    .tabs button { border:0; background:transparent; color:var(--stone-700); padding:7px 9px; border-radius:7px; font:600 12px Inter; cursor:pointer; }
    .tabs button.active { background:var(--olive-200); color:var(--olive-950); box-shadow:inset 0 0 0 1px var(--olive-500); }
    .status { display:flex; align-items:center; gap:7px; color:var(--muted); font-size:12px; white-space:nowrap; }
    .dot { width:9px; height:9px; border-radius:50%; background:var(--green); box-shadow:0 0 0 3px rgba(90,138,94,.22); }
    .ha-chip { display:none; align-items:center; gap:6px; border:1px solid rgba(176,125,42,.65); background:#ead8ad; color:var(--stone-900); border-radius:8px; padding:4px 8px; font:700 11px Inter; }
    .ha-chip.visible { display:inline-flex; }
    .ha-chip button { border:0; background:transparent; color:var(--olive-800); cursor:pointer; padding:0; font:800 12px Inter; }
    .ha-help { display:none; position:absolute; top:44px; left:116px; width:min(360px, calc(100vw - 36px)); background:var(--olive-50); border:1px solid var(--olive-500); border-radius:8px; padding:12px; box-shadow:0 14px 30px rgba(28,25,23,.18); z-index:20; color:var(--stone-900); font:500 12px Inter; }
    .ha-help.visible { display:block; }
    .ha-help strong { display:block; font-size:13px; margin-bottom:6px; }
    .page { width:min(100%, 1840px); margin:0 auto; padding:18px; }
    .hero { background:var(--olive-200); color:var(--stone-900); border:1px solid var(--olive-400); border-radius:8px; padding:18px 20px; display:grid; grid-template-columns:1.2fr 2fr; gap:18px; align-items:end; }
    .hero h1 { font-size:36px; }
    .hero p { color:var(--stone-700); margin:8px 0 0; max-width:620px; }
    .metrics { display:grid; grid-template-columns:repeat(5,minmax(120px,1fr)); gap:10px; margin-top:14px; }
    .card { background:var(--surface); border:1px solid var(--border); border-radius:8px; padding:14px; min-width:0; }
    .metric-label { color:var(--muted); text-transform:uppercase; letter-spacing:.07em; font-size:11px; font-weight:800; }
    .metric-value { font-family:"Instrument Serif",serif; font-size:31px; line-height:1; margin-top:6px; }
    .grid { display:grid; grid-template-columns:minmax(0,1.65fr) minmax(360px,.75fr); gap:12px; margin-top:12px; }
    .summary-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; margin-top:12px; }
    .section-title { display:flex; justify-content:space-between; align-items:end; border-bottom:1px solid var(--border); padding-bottom:8px; margin-bottom:10px; }
    .section-title h2 { font-size:24px; }
    .muted { color:var(--muted); }
    .table-wrap { width:100%; overflow-x:auto; }
    table { width:100%; border-collapse:collapse; font-size:12px; table-layout:auto; }
    .events-table { min-width:1420px; }
    .events-table th:nth-child(1) { width:92px; }
    .events-table th:nth-child(2) { width:170px; }
    .events-table th:nth-child(3) { width:340px; }
    .events-table th:nth-child(4) { width:58px; }
    .events-table th:nth-child(5) { width:96px; }
    .events-table th:nth-child(6) { width:230px; }
    .events-table th:nth-child(7) { width:74px; }
    .events-table th:nth-child(8) { width:auto; min-width:360px; }
    th { color:var(--muted); text-align:left; text-transform:uppercase; letter-spacing:.06em; font-size:10px; border-bottom:1px solid var(--border); padding:7px 6px; }
    td { border-bottom:1px solid rgba(167,174,139,.45); padding:8px 6px; vertical-align:top; }
    td.answer-cell { white-space:normal; overflow-wrap:anywhere; min-width:360px; }
    td.domain-cell, td.rule-cell { white-space:normal; overflow-wrap:anywhere; }
    .list .row strong { min-width:0; overflow-wrap:anywhere; }
    tr:hover td { background:var(--olive-100); }
    tr.clickable { cursor:pointer; }
    tr.selected td { background:var(--olive-100); box-shadow:inset 3px 0 0 var(--olive-700); }
    .badge { display:inline-flex; align-items:center; border-radius:999px; padding:2px 8px; font-size:11px; font-weight:800; text-transform:uppercase; letter-spacing:.04em; }
    .allowed,.static { background:rgba(90,138,94,.14); color:var(--green); }
    .blocked { background:rgba(192,57,43,.14); color:var(--red); }
    .upstream_failed { background:rgba(176,125,42,.17); color:var(--amber); }
    .cached { background:rgba(78,141,163,.14); color:var(--blue); }
    .toolbar { display:flex; flex-wrap:wrap; gap:8px; align-items:center; margin-bottom:10px; }
    input, select { border:1px solid var(--border); background:var(--olive-50); color:var(--text); border-radius:7px; padding:8px 10px; font:inherit; min-height:36px; }
    button.primary, button.secondary, button.danger { border:0; border-radius:7px; min-height:36px; padding:8px 12px; font:700 12px Inter; cursor:pointer; }
    button.primary { background:var(--olive-100); color:var(--olive-950); border:1px solid var(--olive-600); }
    button.secondary { background:var(--surface2); color:var(--olive-900); border:1px solid var(--border); }
    button.danger { background:rgba(192,57,43,.14); color:var(--red); border:1px solid rgba(192,57,43,.35); }
    .button-link { display:inline-flex; align-items:center; min-height:36px; padding:8px 12px; border-radius:7px; background:var(--olive-100); color:var(--olive-950); border:1px solid var(--olive-600); font:700 12px Inter; text-decoration:none; }
    button.compact { min-height:28px; padding:4px 8px; font-size:11px; white-space:nowrap; }
    .hidden { display:none; }
    .mono { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:12px; }
    .list { display:flex; flex-direction:column; gap:8px; }
    .row { display:flex; align-items:center; justify-content:space-between; gap:10px; }
    .bar { height:8px; background:var(--olive-100); border-radius:99px; overflow:hidden; margin-top:5px; }
    .bar span { display:block; height:100%; background:var(--olive-700); }
    .list-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(260px,1fr)); gap:10px; }
    .source-card { color:var(--stone-900); }
    .source-card h3 { font-family:Inter,system-ui,sans-serif; font-size:14px; margin:0 0 5px; color:var(--stone-900); }
    .source-card p { margin:0 0 10px; color:var(--stone-700); min-height:48px; }
    .source-card.disabled { opacity:.68; }
    .source-card a { color:var(--olive-800); font-weight:800; text-decoration:none; }
    .source-card a:hover { text-decoration:underline; }
    .source-links { display:flex; flex-wrap:wrap; gap:8px; margin-top:10px; }
    .source-links a { display:inline-flex; align-items:center; min-height:28px; border:1px solid var(--border); border-radius:7px; padding:4px 8px; background:var(--olive-50); font-size:12px; }
    .switch { position:relative; display:inline-flex; align-items:center; gap:8px; cursor:pointer; font-size:12px; font-weight:800; color:var(--muted); }
    .source-card .switch { color:var(--stone-700); }
    .source-card .muted { color:var(--stone-700); }
    .source-card .mono { color:var(--stone-700); }
    .switch input { position:absolute; opacity:0; pointer-events:none; }
    .slider { width:38px; height:22px; border-radius:999px; background:var(--stone-300); position:relative; transition:.16s; border:1px solid var(--border); }
    .slider::after { content:""; position:absolute; width:16px; height:16px; left:2px; top:2px; background:white; border-radius:50%; transition:.16s; box-shadow:0 1px 2px rgba(0,0,0,.25); }
    .switch input:checked + .slider { background:var(--olive-700); }
    .switch input:checked + .slider::after { transform:translateX(16px); }
    @media (max-width: 860px) { .hero,.grid,.summary-grid { grid-template-columns:1fr; } .metrics { grid-template-columns:repeat(2,1fr); } .topnav-inner { height:auto; align-items:flex-start; padding-block:10px; flex-direction:column; } .tabs { justify-content:flex-start; } }
  </style>
</head>
<body>
  <nav class="topnav">
    <div class="topnav-inner">
      <div class="brand"><span class="brand-mark">DNS</span><span>TM-DNS</span><span class="status"><span class="dot"></span><span id="statusText">starting</span></span><span id="haChip" class="ha-chip"><span>!</span><span>No secondary DNS</span><button type="button" onclick="toggleHAHelp()">i</button></span><div id="haHelp" class="ha-help"></div></div>
      <div class="tabs" id="tabs"></div>
    </div>
  </nav>
  <main class="page">
    <section class="hero">
      <div>
        <h1>TM-DNS</h1>
        <p>All your queries are belong to us</p>
      </div>
      <div class="metrics" id="metrics"></div>
    </section>
    <section id="view"></section>
  </main>
<script>
const pages = ["Dashboard","Realtime","Blocked","Hosts","Rules","Lists","Records","Reports","Audit","Load","Diagnostics","Settings"];
const pageLabels = {"Lists":"Block Lists"};
let initialHostID = null;
function pageFromHash() {
  const raw = decodeURIComponent((window.location.hash || "").replace(/^#/, ""));
  if (raw.startsWith("Host:")) {
    const id = Number(raw.replace("Host:", ""));
    if (id > 0) initialHostID = id;
    return "Host Detail";
  }
  return pages.includes(raw) ? raw : "Dashboard";
}
const initialPage = pageFromHash();
let state = { page:initialPage, authed:false, dashboard:null, realtime:[], blocked:[], hosts:[], rules:[], records:[], audit:[], hostReport:null, hostDetail:null, selectedHostID:initialHostID, unifi:null, retention:null, diagnostics:null, ha:null, haJoinRequests:[], lastFullLoad:0 };
let blocklistPresets = [];
let blocklistSources = [];
const $ = s => document.querySelector(s);
const esc = v => String(v ?? "").replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const cleanDomain = v => String(v ?? "").trim().replace(/\.$/, "").toLowerCase();
const api = async (url, opts) => {
  const res = await fetch(url, opts);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
};
function loginView(message = "") {
  $("#metrics").innerHTML = "";
  $("#statusText").textContent = "login required";
  $("#tabs").innerHTML = "";
  $("#view").innerHTML = '<div class="card" style="margin-top:12px;max-width:620px"><div class="section-title"><h2>Admin Login</h2><span class="muted">required for LAN dashboard</span></div><div class="toolbar"><input id="adminToken" type="password" placeholder="Admin token" style="min-width:360px"><button class="primary" onclick="login()">Login</button></div><p class="muted">The token is stored on the TM-DNS Mac in <span class="mono">admin-token.txt</span> next to the database, unless <span class="mono">TMDNS_ADMIN_TOKEN</span> is set.</p>'+(message ? '<p style="color:var(--red)">'+esc(message)+'</p>' : '')+'</div>';
}
function fmtTime(v) { if (!v) return ""; return new Date(v).toLocaleTimeString(); }
function badge(v) { return '<span class="badge '+v+'">'+v.replace("_"," ")+'</span>'; }
function renderTabs() {
  $("#tabs").innerHTML = pages.map(p => '<button class="'+(state.page===p?'active':'')+'" onclick="show(\''+p+'\')">'+(pageLabels[p]||p)+'</button>').join("");
}
function renderMetrics() {
  const d = state.dashboard?.dashboard || {};
  const dns = state.dashboard?.dns || {};
  const v = state.dashboard?.version || {};
  $("#statusText").textContent = dns.dns_addr ? 'healthy on '+dns.dns_addr+' · '+(v.version||'dev') : 'loading';
  renderHAChip();
  $("#metrics").innerHTML = [
    ["Queries Today", d.queries_today || 0],
    ["Blocked Today", d.blocked_today || 0],
    ["Unique Hosts", d.unique_hosts || 0],
    ["Runtime Queries", dns.queries || 0],
    ["Dropped Events", dns.dropped_events || 0],
  ].map(x => '<div class="card"><div class="metric-label">'+x[0]+'</div><div class="metric-value">'+x[1]+'</div></div>').join("");
}
function haWarningDetail() {
  const ha = state.dashboard?.ha;
  if (!ha) return "TM-DNS cannot confirm secondary DNS health yet. A single DNS server can take the network offline if this Mac is restarted, asleep, disconnected, or updating.";
  if (!ha.enabled) return "Secondary DNS is not enabled. Put a second onsite TM-DNS server in DHCP as backup DNS before relying on this for production.";
  if (!ha.configured) return "Secondary DNS is enabled but peer URL or peer token is missing. Complete peer setup, then run Heartbeat and Push Sync.";
  if (ha.stale) return "Secondary DNS heartbeat is stale. Check the peer Mac, network path, admin token, and firewall access before making network-wide DNS changes.";
  return "";
}
function renderHAChip() {
  const detail = haWarningDetail();
  const chip = $("#haChip");
  const help = $("#haHelp");
  if (!chip || !help) return;
  chip.classList.toggle("visible", !!detail);
  if (!detail) {
    help.classList.remove("visible");
    help.innerHTML = "";
    return;
  }
  help.innerHTML = '<strong>Redundant DNS</strong><p style="margin:0 0 8px">'+esc(detail)+'</p><p class="muted" style="margin:0">Use two wired Macs with static IPs or DHCP reservations. Configure one as Primary, one as Secondary, run Heartbeat, Push Sync from the Primary, then advertise both IPs as DNS servers in DHCP.</p>';
}
function toggleHAHelp() {
  renderHAChip();
  $("#haHelp")?.classList.toggle("visible");
}
function applianceWarning() {
  const p = state.dashboard?.system?.power || {};
  if (!p.supported || !p.sleep_configured) return "";
  return '<div class="card" style="margin-top:12px;border-color:rgba(176,125,42,.55);background:#eee5d1"><div class="section-title"><h2>Appliance Mode Warning</h2><span class="muted mono">'+esc(p.profile||'power')+'</span></div><p style="margin-top:0;color:var(--stone-900)">This Mac is still configured to sleep after '+esc(p.system_sleep_minutes)+' minutes. If it sleeps, TM-DNS can stop answering DNS for the network.</p><div class="toolbar"><a class="button-link" href="x-apple.systempreferences:com.apple.Battery-Settings.extension">Open macOS Power Settings</a><span class="muted mono">Terminal: sudo pmset -a sleep 0</span></div></div>';
}
function eventsTable(events) {
  return '<div class="table-wrap"><table class="events-table"><thead><tr><th>Time</th><th>Host</th><th>Domain</th><th>Type</th><th>Action</th><th>Rule/List</th><th>Latency</th><th>Answer</th></tr></thead><tbody>'+
    events.map(e => '<tr><td class="mono">'+fmtTime(e.timestamp)+'</td><td>'+e.host_label+'<div class="muted mono">'+e.source_ip+'</div></td><td class="mono domain-cell">'+e.query_name+'</td><td>'+e.query_type+'</td><td>'+badge(e.action)+'</td><td class="mono rule-cell">'+(e.matched_source||'')+'</td><td>'+e.latency_ms+'ms</td><td class="mono answer-cell">'+(e.answer_summary||e.response_code)+'</td></tr>').join("")+
    '</tbody></table></div>';
}
function hostDisplayName(h) {
  return esc(h?.label || h?.hostname || h?.source_ip || h?.key || 'host');
}
function renderHostInvestigation() {
  const detail = state.hostDetail;
  if (!detail) return '<div class="card"><div class="section-title"><h2>Host Detail</h2><span class="muted">select a host</span></div><p class="muted">Click a host to see top sites, blocked attempts, and the chronological DNS request log.</p></div>';
  const h = detail.host || {};
  return '<div class="card"><div class="section-title"><h2>'+hostDisplayName(h)+'</h2><span class="muted mono">'+esc(h.source_ip||'')+'</span></div>'+
    '<div class="metrics" style="grid-template-columns:repeat(3,minmax(120px,1fr));margin:0 0 12px"><div><div class="metric-label">Queries</div><div class="metric-value">'+(h.query_count||0)+'</div></div><div><div class="metric-label">Blocked</div><div class="metric-value">'+(h.block_count||0)+'</div></div><div><div class="metric-label">Identity</div><div class="metric-value" style="font-size:24px">'+esc(h.identity_confidence||'source_ip')+'</div></div></div>'+
    '<p class="muted mono" style="margin-top:0">DNS '+esc(h.hostname||'not learned')+' · MAC '+esc(h.mac||'not learned')+(h.vendor ? ' · '+esc(h.vendor) : '')+'</p>'+
    '<div class="section-title"><h2>Top Sites</h2><span class="muted">click Block to create policy</span></div>'+topList(detail.top_domains||[], {block:true})+
    '<div class="section-title" style="margin-top:12px"><h2>Request Timeline</h2><span class="muted">latest 100 requests</span></div>'+eventsTable(detail.recent||[])+
    '<div class="section-title" style="margin-top:12px"><h2>Blocked Timeline</h2><span class="muted">filtered attempts</span></div>'+eventsTable(detail.blocked||[])+'</div>';
}
function renderHostDetailPage() {
  const detail = state.hostDetail;
  if (!detail) return '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Host Detail</h2><span class="muted">loading</span></div><p class="muted">Select a host from the Hosts page.</p></div>';
  const h = detail.host || {};
  const recent = detail.recent || [];
  const lastSeen = recent.length ? fmtTime(recent[0].timestamp) : esc(h.last_seen || 'no activity');
  return '<div class="card" style="margin-top:12px"><div class="section-title"><h2>'+hostDisplayName(h)+'</h2><span class="muted mono">'+esc(h.source_ip||'')+'</span></div>'+
    '<div class="toolbar"><button class="secondary" onclick="show(\'Hosts\')">Back to Hosts</button><span class="muted">Live refreshes every 5 seconds while this page is open.</span></div>'+
    '<div class="metrics" style="grid-template-columns:repeat(4,minmax(140px,1fr));margin:0 0 12px"><div><div class="metric-label">Total Queries</div><div class="metric-value">'+(h.query_count||0)+'</div></div><div><div class="metric-label">Total Blocks</div><div class="metric-value">'+(h.block_count||0)+'</div></div><div><div class="metric-label">Last Hit</div><div class="metric-value" style="font-size:24px">'+lastSeen+'</div></div><div><div class="metric-label">Identity</div><div class="metric-value" style="font-size:24px">'+esc(h.identity_confidence||'source_ip')+'</div></div></div>'+
    '<p class="muted mono" style="margin-top:0">DNS '+esc(h.hostname||'not learned')+' · MAC '+esc(h.mac||'not learned')+(h.vendor ? ' · '+esc(h.vendor) : '')+'</p></div>'+
    '<div class="summary-grid"><div class="card"><div class="section-title"><h2>Top Domains</h2><span class="muted">selected window, sorted by hits</span></div>'+topList(detail.top_domains||[], {block:true})+'</div>'+
    '<div class="card"><div class="section-title"><h2>Actions</h2><span class="muted">last 24 hours</span></div>'+topList(detail.top_actions||[])+'</div></div>'+
    '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Realtime Timeline</h2><span class="muted">most recent hits first</span></div>'+eventsTable(recent)+'</div>';
}
function topList(rows, opts = {}) {
  const max = Math.max(1, ...rows.map(r => r.count));
  return '<div class="list">'+rows.map(r => {
    const key = cleanDomain(r.key);
    const action = opts.block ? '<button class="danger compact" data-domain="'+esc(key)+'" onclick="blockDomain(this.dataset.domain)">Block</button>' : '';
    const percent = Number.isFinite(r.percent) ? '<span class="muted mono">'+Math.round(r.percent)+'%</span>' : '';
    return '<div class="card"><div class="row"><strong class="mono">'+esc(r.key)+'</strong><div class="row" style="gap:8px"><span>'+r.count+'</span>'+percent+action+'</div></div><div class="bar"><span style="width:'+(r.count/max*100)+'%"></span></div></div>';
  }).join("")+'</div>';
}
function topHostsList(rows) {
  const max = Math.max(1, ...rows.map(r => r.count));
  return '<div class="list">'+rows.map(r => {
    const name = r.label || r.hostname || r.source_ip || r.key;
    const dnsName = r.hostname || 'hostname not learned yet';
    const macLine = r.mac ? '<div class="muted mono">'+esc(r.mac)+(r.vendor ? ' · '+esc(r.vendor) : '')+'</div>' : '';
    return '<div class="card"><div class="row"><div style="min-width:0"><strong class="mono">'+esc(name)+'</strong><div class="muted mono">'+esc(dnsName)+'</div><div class="muted mono">'+esc(r.source_ip || '')+'</div>'+macLine+'</div><span>'+r.count+'</span></div><div class="bar"><span style="width:'+(r.count/max*100)+'%"></span></div></div>';
  }).join("")+'</div>';
}
function systemCards() {
  const s = state.dashboard?.system || {};
  return '<div class="card" style="margin-top:12px"><div class="section-title"><h2>App Load</h2><span class="muted">process and storage</span></div><div class="metrics" style="grid-template-columns:repeat(4,minmax(120px,1fr));margin-top:0">'+
    [['CPU', (s.cpu_percent ?? 0)+'%'], ['Memory', (s.resident_mb ?? 0)+' MB'], ['TM-DNS Storage', (s.app_storage_mb ?? 0)+' MB'], ['System Disk Used', (s.disk_used_percent ?? 0)+'%']].map(x => '<div><div class="metric-label">'+x[0]+'</div><div class="metric-value">'+x[1]+'</div></div>').join("")+
    '</div><p class="muted mono" style="margin-bottom:0">TM-DNS: DB '+(s.db_size_mb ?? 0)+' MB · WAL '+(s.wal_size_mb ?? 0)+' MB · SHM '+(s.shm_size_mb ?? 0)+' MB</p><p class="muted mono" style="margin:4px 0 0">System disk: '+(s.disk_used_gb ?? 0)+' GB used / '+(s.disk_total_gb ?? 0)+' GB · '+(s.disk_free_gb ?? 0)+' GB free · data '+(s.data_dir||'')+'</p></div>';
}
function toggleSwitch(checked, onChange) {
  return '<label class="switch"><input type="checkbox" '+(checked?'checked':'')+' onchange="'+onChange+'"><span class="slider"></span><span>'+(checked?'Enabled':'Disabled')+'</span></label>';
}
function blocklistCards() {
  return '<div class="list-grid">'+blocklistPresets.map(s => '<div class="card source-card '+(s.enabled?'':'disabled')+'"><div class="row"><h3>'+s.name+'</h3>'+toggleSwitch(s.enabled, "togglePreset('"+s.id+"', this.checked)")+'</div><div class="row"><span class="badge static">'+s.tier+'</span><span class="muted mono">'+s.id+'</span></div><p>'+s.description+'</p><div class="source-links"><a href="'+s.home_url+'" target="_blank" rel="noopener noreferrer">Review</a><a href="'+s.source_url+'" target="_blank" rel="noopener noreferrer">Source</a></div></div>').join("")+'</div>';
}
function blocklistSourceCards() {
  const cards = blocklistSources.map(s => '<div class="card source-card '+(s.enabled?'':'disabled')+'"><div class="row"><h3>'+esc(s.name)+'</h3>'+toggleSwitch(s.enabled, "toggleSource("+s.id+", this.checked)")+'</div><div class="row"><span class="badge static">'+esc(s.format)+'</span><span class="muted mono">'+esc(s.last_status||'not checked')+'</span></div><p class="mono">'+esc(s.url)+'</p></div>').join("");
  return cards || '<p class="muted">No custom sources yet.</p>';
}
function render() {
  renderTabs(); renderMetrics();
  const d = state.dashboard?.dashboard || {};
  if (state.page === "Dashboard") $("#view").innerHTML = applianceWarning()+systemCards()+'<div class="card" style="margin-top:12px"><div class="section-title"><h2>Realtime Activity</h2><span class="muted">latest DNS decisions</span></div>'+eventsTable(d.recent||[])+'</div><div class="summary-grid"><div class="card"><div class="section-title"><h2>Top Hosts</h2><span class="muted">name, DNS name, IP</span></div>'+topHostsList(d.top_hosts||[])+'</div><div class="card"><div class="section-title"><h2>Top Domains</h2><span class="muted">last 48 hours, one-click policy</span></div>'+topList(d.top_domains||[], {block:true})+'</div></div>';
  if (state.page === "Realtime") $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Realtime Firewall View</h2><span class="muted">auto-refreshes every 5s</span></div>'+eventsTable(state.realtime)+'</div>';
  if (state.page === "Blocked") $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Blocked Attempts</h2><span class="muted">who, what, why, when</span></div>'+eventsTable(state.blocked)+'</div>';
  if (state.page === "Hosts") $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Hosts</h2><span class="muted">click a host to open its detail view</span></div><div class="table-wrap"><table><thead><tr><th>Host</th><th>IP</th><th>MAC / Vendor</th><th>Identity</th><th>Last Seen</th><th>Queries</th><th>Blocks</th></tr></thead><tbody>'+state.hosts.map(h => '<tr class="clickable '+(state.selectedHostID===h.id?'selected':'')+'" onclick="selectHost('+h.id+')"><td><strong>'+esc(h.label||h.hostname||h.source_ip)+'</strong><div class="muted mono">'+esc(h.hostname||'hostname not learned yet')+'</div></td><td class="mono">'+esc(h.source_ip)+'</td><td class="mono">'+esc(h.mac||'')+'<div class="muted">'+esc(h.vendor||'')+'</div></td><td>'+esc(h.identity_confidence)+'<div class="muted mono">'+esc(h.identity_last_checked||'not checked yet')+'</div></td><td>'+h.last_seen+'</td><td>'+h.query_count+'</td><td>'+h.block_count+'</td></tr>').join("")+'</tbody></table></div></div>';
  if (state.page === "Host Detail") $("#view").innerHTML = renderHostDetailPage();
  if (state.page === "Rules") $("#view").innerHTML = '<div class="grid"><div class="card"><div class="section-title"><h2>Rules</h2><span class="muted">firewall-style DNS policy</span></div><div class="toolbar"><input id="ruleTarget" placeholder="domain.example"><button class="primary" onclick="addRule(\'block\')">Block</button><button class="secondary" onclick="addRule(\'allow\')">Allow</button></div><table><thead><tr><th>ID</th><th>Target</th><th>Action</th><th>Status</th><th>Hits</th><th>Last Hit</th><th>Note</th></tr></thead><tbody>'+state.rules.map(r => '<tr><td>'+r.id+'</td><td class="mono">'+r.target+'</td><td>'+badge(r.action==="block"?"blocked":"allowed")+'</td><td>'+toggleSwitch(r.enabled, "toggleRule("+r.id+", this.checked)")+'</td><td>'+r.hit_count+'</td><td>'+fmtTime(r.last_hit_at)+'</td><td>'+r.note+'</td></tr>').join("")+'</tbody></table></div><div class="card"><div class="section-title"><h2>Public Block Lists</h2><span class="muted">enable later ingestion targets</span></div><p class="muted" style="margin-top:0">Enable block lists here, then refresh them from the Block Lists page to compile local DNS enforcement entries.</p>'+blocklistCards()+'</div></div>';
  if (state.page === "Lists") $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Block Lists</h2><span class="muted">enabled block lists are enforced after refresh</span></div><div class="toolbar"><button class="primary" onclick="refreshLists()">Refresh Enabled Block Lists</button><span class="muted">Downloads, parses, and atomically swaps compiled DNS blocklist entries.</span></div></div><div class="grid"><div class="card"><div class="section-title"><h2>Custom Sources</h2><span class="muted">raw GitHub or public list URL</span></div><div class="toolbar"><input id="srcName" placeholder="Name"><input id="srcURL" placeholder="https://raw.githubusercontent.com/org/repo/main/domains.txt" style="min-width:360px"><select id="srcFormat"><option value="domains">domains</option><option value="hosts">hosts</option><option value="adguard">adguard</option></select><button class="primary" onclick="addSource()">Add Source</button></div><div class="list-grid">'+blocklistSourceCards()+'</div></div><div class="card"><div class="section-title"><h2>Curated Presets</h2><span class="muted">review and enable</span></div>'+blocklistCards()+'</div></div>';
  if (state.page === "Records") $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Static Records</h2><span class="muted">local DNS records</span></div><div class="toolbar"><input id="recName" placeholder="name.test"><select id="recType"><option>A</option><option>AAAA</option><option>CNAME</option><option>TXT</option></select><input id="recValue" placeholder="value"><input id="recTTL" type="number" value="60" style="width:90px"><button class="primary" onclick="addRecord()">Save Record</button></div><table><thead><tr><th>Name</th><th>Type</th><th>Value</th><th>TTL</th></tr></thead><tbody>'+state.records.map(r => '<tr><td class="mono">'+r.name+'</td><td>'+r.type+'</td><td class="mono">'+r.value+'</td><td>'+r.ttl+'</td></tr>').join("")+'</tbody></table></div>';
  if (state.page === "Reports") { const r = state.hostReport; $("#view").innerHTML = '<div class="grid"><div class="card"><div class="section-title"><h2>Host Report</h2><span class="muted">'+(r?.host?.label||r?.host?.source_ip||'no host yet')+'</span></div>'+(r ? '<div class="metrics" style="grid-template-columns:repeat(3,1fr);margin:0 0 12px"><div><div class="metric-label">Queries</div><div class="metric-value">'+r.total_queries+'</div></div><div><div class="metric-label">Blocked</div><div class="metric-value">'+r.total_blocked+'</div></div><div><div class="metric-label">Domains</div><div class="metric-value">'+r.unique_domains+'</div></div></div><div class="section-title"><h2>Top Domains</h2><span class="muted">one-click policy</span></div>'+topList(r.top_domains||[], {block:true})+'<div class="section-title" style="margin-top:12px"><h2>Notes</h2></div><ul>'+r.recommended_notes.map(n => '<li>'+n+'</li>').join("")+'</ul>' : '<p class="muted">No host activity has been recorded yet.</p>')+'</div><div class="card"><div class="section-title"><h2>Policy Report</h2></div>'+topList(d.rule_hits||[])+'</div></div>'; }
  if (state.page === "Audit") $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Audit Log</h2><span class="muted">policy, list, and admin changes</span></div><div class="table-wrap"><table><thead><tr><th>Time</th><th>Action</th><th>Target</th><th>Detail</th></tr></thead><tbody>'+state.audit.map(e => '<tr><td class="mono">'+fmtTime(e.timestamp)+'</td><td class="mono">'+esc(e.action)+'</td><td class="mono domain-cell">'+esc(e.target)+'</td><td>'+esc(e.detail)+'</td></tr>').join("")+'</tbody></table></div></div>';
  if (state.page === "Load") { const dns = state.dashboard?.dns || {}; const sys = state.dashboard?.system || {}; $("#view").innerHTML = '<div class="grid"><div class="card"><div class="section-title"><h2>Service Load</h2></div><table><tbody>'+Object.entries(dns).map(([k,v]) => '<tr><th>'+k+'</th><td class="mono">'+JSON.stringify(v)+'</td></tr>').join("")+'</tbody></table></div><div class="card"><div class="section-title"><h2>System</h2></div><table><tbody>'+Object.entries(sys).map(([k,v]) => '<tr><th>'+k+'</th><td class="mono">'+JSON.stringify(v)+'</td></tr>').join("")+'</tbody></table></div></div>'; }
  if (state.page === "Diagnostics") { const x = state.diagnostics || {}; const warnings = x.warnings || []; $("#view").innerHTML = '<div class="card" style="margin-top:12px"><div class="section-title"><h2>Diagnostics</h2><span class="muted">'+(x.ok?'ready':'needs attention')+'</span></div>'+(warnings.length ? '<ul>'+warnings.map(w => '<li>'+esc(w)+'</li>').join("")+'</ul>' : '<p class="muted">No current diagnostic warnings.</p>')+'<div class="toolbar"><button class="secondary" onclick="copyDiagnostics()">Copy JSON</button><span class="muted mono">Use this for support and deployment checks.</span></div></div><div class="grid"><div class="card"><div class="section-title"><h2>Config</h2></div><table><tbody>'+Object.entries(x.config||{}).map(([k,v]) => '<tr><th>'+esc(k)+'</th><td class="mono">'+esc(JSON.stringify(v))+'</td></tr>').join("")+'</tbody></table></div><div class="card"><div class="section-title"><h2>DNS</h2></div><table><tbody>'+Object.entries(x.dns||{}).map(([k,v]) => '<tr><th>'+esc(k)+'</th><td class="mono">'+esc(JSON.stringify(v))+'</td></tr>').join("")+'</tbody></table></div><div class="card"><div class="section-title"><h2>Secondary DNS</h2></div><table><tbody>'+Object.entries(x.ha||{}).map(([k,v]) => '<tr><th>'+esc(k)+'</th><td class="mono">'+esc(JSON.stringify(v))+'</td></tr>').join("")+'</tbody></table></div></div>'; }
  if (state.page === "Settings") { const u = state.unifi || {}; const r = state.retention || {}; const ha = state.ha || {}; $("#view").innerHTML = '<div class="grid"><div class="card"><div class="section-title"><h2>UniFi Identity Import</h2><span class="muted">optional, never required</span></div><div class="toolbar"><label class="switch"><input id="unifiEnabled" type="checkbox" '+(u.enabled?'checked':'')+'><span class="slider"></span><span>Enabled</span></label></div><div class="toolbar"><input id="unifiBaseURL" placeholder="https://unifi.example.com" value="'+esc(u.base_url||'')+'" style="min-width:320px"><input id="unifiSite" placeholder="default" value="'+esc(u.site||'default')+'" style="width:130px"><input id="unifiAPIKey" type="password" placeholder="'+(u.has_api_key?'API key saved - leave blank to keep':'API key')+'" style="min-width:260px"><button class="primary" onclick="saveUniFi()">Save</button><button class="secondary" onclick="testUniFi()">Test</button><button class="secondary" onclick="importUniFi()">Import Clients</button></div><p class="muted">Create a read-only UniFi API key, enter the UniFi OS/controller URL and site name, then test before importing. The key is encrypted locally before storage.</p><p class="muted mono">Status: '+esc(u.last_status||'not configured')+' · Last import: '+esc(u.last_import||'never')+'</p></div><div class="card"><div class="section-title"><h2>Onsite Secondary DNS</h2><span class="muted">heartbeat and manual policy sync</span></div><div class="toolbar"><label class="switch"><input id="haEnabled" type="checkbox" '+(ha.enabled?'checked':'')+'><span class="slider"></span><span>Enabled</span></label><select id="haRole"><option value="primary" '+((ha.role||'primary')==='primary'?'selected':'')+'>Primary</option><option value="secondary" '+(ha.role==='secondary'?'selected':'')+'>Secondary</option></select></div><div class="toolbar"><input id="haPeerName" placeholder="Secondary DNS" value="'+esc(ha.peer_name||'')+'"><input id="haPeerURL" placeholder="https://tm-dns-secondary.example.edu:8080" value="'+esc(ha.peer_url||'')+'" style="min-width:300px"><input id="haPeerToken" type="password" placeholder="'+(ha.has_peer_token?'Peer token saved - leave blank to keep':'Peer admin token')+'" style="min-width:240px"></div><div class="toolbar"><button class="primary" onclick="saveHA()">Save Peer</button><button class="secondary" onclick="testHA()">Heartbeat</button><button class="secondary" onclick="syncHA()">Push Sync</button></div><p class="muted">Put both onsite DNS server IPs into DHCP DNS servers. Push Sync copies static records, rules, block list settings, custom list sources, and retention settings to the peer.</p><p class="muted mono">Status: '+esc(ha.last_status||'not configured')+' · Heartbeat: '+esc(ha.last_heartbeat||'never')+' · Sync: '+esc(ha.last_sync||'never')+'</p></div><div class="card"><div class="section-title"><h2>Retention</h2><span class="muted">query log storage</span></div><div class="toolbar"><input id="retentionDays" type="number" min="1" max="3650" value="'+esc(r.days||30)+'" style="width:120px"><button class="primary" onclick="saveRetention()">Save Days</button><button class="secondary" onclick="purgeRetention()">Purge Now</button></div><p class="muted mono">Last purge: '+esc(r.last_purge||'never')+'</p><div class="section-title" style="margin-top:12px"><h2>Instructions</h2></div><ol><li>Use the UniFi console URL, usually https://gateway-ip or https://unifi-host:8443.</li><li>Leave site as default unless your controller uses another site id.</li><li>If a site does not use UniFi, leave this blank. PTR, Bonjour, and ARP discovery continue to work.</li></ol></div></div>'; }
}
async function load(full = true) {
  const auth = await api('/api/auth/status');
  if (!auth.authenticated) {
    state.authed = false;
    loginView();
    return;
  }
  state.authed = true;
  state.dashboard = await api('/api/dashboard');
  state.realtime = await api('/api/realtime');
  state.blocked = await api('/api/blocked');
  if (full || Date.now() - state.lastFullLoad > 60000) {
    state.hosts = await api('/api/hosts');
    state.rules = await api('/api/rules');
    state.records = await api('/api/records');
    state.audit = await api('/api/audit');
    state.unifi = await api('/api/settings/unifi');
    state.retention = await api('/api/settings/retention');
    state.diagnostics = await api('/api/diagnostics');
    state.ha = await api('/api/ha/settings');
    state.haJoinRequests = await api('/api/ha/join-requests');
    blocklistPresets = await api('/api/blocklist-presets');
    blocklistSources = await api('/api/blocklist-sources');
    state.hostReport = state.selectedHostID ? await api('/api/reports/host/'+state.selectedHostID) : null;
    state.lastFullLoad = Date.now();
  }
  if (state.page === "Host Detail" && state.selectedHostID) {
    state.hostDetail = await api('/api/hosts/'+state.selectedHostID);
  } else if (full && state.selectedHostID) {
    state.hostDetail = await api('/api/hosts/'+state.selectedHostID);
  }
  render();
}
async function login() {
  try {
    await api('/api/auth/login', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({token:$("#adminToken").value})});
    await load();
  } catch(e) {
    loginView("Invalid token");
  }
}
function show(p) {
  state.page = pages.includes(p) || p === "Host Detail" ? p : "Dashboard";
  const hash = state.page === "Host Detail" && state.selectedHostID ? "Host:"+state.selectedHostID : state.page;
  if (window.location.hash !== "#"+hash) {
    history.replaceState(null, "", "#"+hash);
  }
  render();
}
window.addEventListener("hashchange", async () => {
  const raw = decodeURIComponent((window.location.hash || "").replace(/^#/, ""));
  if (raw.startsWith("Host:")) {
    const id = Number(raw.replace("Host:", ""));
    if (id > 0) {
      state.selectedHostID = id;
      state.hostDetail = await api('/api/hosts/'+id);
      state.hostReport = await api('/api/reports/host/'+id);
    }
  }
  state.page = pageFromHash();
  render();
});
async function addRule(action) {
  const target = cleanDomain($("#ruleTarget").value);
  await api('/api/rules/'+action, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({target, note:'created from UI'})});
  await load();
}
async function blockDomain(domain) {
  const target = cleanDomain(domain);
  if (!target) return;
  await api('/api/rules/block', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({target, note:'blocked from Top Domains'})});
  await load();
}
async function selectHost(id) {
  state.selectedHostID = id;
  state.hostDetail = await api('/api/hosts/'+id);
  state.hostReport = await api('/api/reports/host/'+id);
  state.page = "Host Detail";
  history.replaceState(null, "", "#Host:"+id);
  render();
}
async function toggleRule(id, enabled) {
  await api('/api/rules/'+id, {method:'PATCH', headers:{'Content-Type':'application/json'}, body:JSON.stringify({enabled})});
  await load();
}
async function togglePreset(id, enabled) {
  await api('/api/blocklist-presets/'+id, {method:'PATCH', headers:{'Content-Type':'application/json'}, body:JSON.stringify({enabled})});
  await load();
}
async function toggleSource(id, enabled) {
  await api('/api/blocklist-sources/'+id, {method:'PATCH', headers:{'Content-Type':'application/json'}, body:JSON.stringify({enabled})});
  await load();
}
async function addSource() {
  await api('/api/blocklist-sources', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name:$("#srcName").value, url:$("#srcURL").value, format:$("#srcFormat").value})});
  await load();
}
async function refreshLists() {
  const results = await api('/api/blocklists/refresh', {method:'POST'});
  alert(results.map(r => r.source_name+': '+r.status+' ('+(r.entries||0)+' entries)').join('\n') || 'No enabled lists.');
  await load();
}
async function addRecord() {
  await api('/api/records', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name:$("#recName").value, type:$("#recType").value, value:$("#recValue").value, ttl:Number($("#recTTL").value||60)})});
  await load();
}
async function saveUniFi() {
  state.unifi = await api('/api/settings/unifi', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({enabled:$("#unifiEnabled").checked, base_url:$("#unifiBaseURL").value, site:$("#unifiSite").value, api_key:$("#unifiAPIKey").value})});
  await load();
}
async function testUniFi() {
  await saveUniFi();
  const result = await api('/api/settings/unifi/test', {method:'POST'});
  alert(result.status === 'ok' ? 'Connected. Saw '+result.seen+' clients.' : 'UniFi test failed: '+result.error);
  await load();
}
async function importUniFi() {
  await saveUniFi();
  const result = await api('/api/settings/unifi/import', {method:'POST'});
  alert(result.status === 'ok' ? 'Imported '+result.updated+' of '+result.seen+' clients.' : 'UniFi import failed: '+result.error);
  await load();
}
async function saveRetention() {
  state.retention = await api('/api/settings/retention', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({days:Number($("#retentionDays").value||30)})});
  await load();
}
async function purgeRetention() {
  const result = await api('/api/settings/retention/purge', {method:'POST'});
  alert('Purged '+result.removed+' old query events.');
  await load();
}
async function saveHA() {
  state.ha = await api('/api/ha/settings', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({enabled:$("#haEnabled").checked, role:$("#haRole").value, peer_name:$("#haPeerName").value, peer_url:$("#haPeerURL").value, peer_token:$("#haPeerToken").value})});
  await load();
}
async function requestHAJoin() {
  const result = await api($("#joinPrimaryURL").value.replace(/\/$/, '')+'/api/ha/join-requests', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({node_name:$("#joinNodeName").value || 'TM-DNS Secondary', node_url:$("#joinLocalURL").value, node_role:'secondary', node_version:(state.dashboard?.version?.version || 'unknown'), requester_token:$("#joinToken").value})});
  alert('Join request sent: '+result.status);
}
async function acceptHAJoin(id) {
  const result = await api('/api/ha/join-requests/accept', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id})});
  alert(result.secondary_configured ? 'Join accepted and secondary configured.' : 'Join accepted. Secondary warning: '+(result.warning||'manual configuration may be needed'));
  await load();
}
async function testHA() {
  await saveHA();
  const result = await api('/api/ha/test', {method:'POST'});
  alert(result.status === 'ok' ? 'Peer heartbeat OK: '+(result.peer_version||'version unknown') : 'Peer heartbeat failed: '+(result.error||result.status));
  await load();
}
async function syncHA() {
  await saveHA();
  const result = await api('/api/ha/sync', {method:'POST'});
  alert(result.status === 'ok' ? 'Synced '+result.rules+' rules, '+result.static_records+' records, '+result.blocklist_sources+' custom lists.' : 'Peer sync failed: '+(result.error||result.status));
  await load();
}
async function copyDiagnostics() {
  await navigator.clipboard.writeText(JSON.stringify(state.diagnostics || {}, null, 2));
}
load().catch(e => { $("#view").innerHTML = '<div class="card" style="margin-top:12px;color:var(--red)">'+e.message+'</div>'; });
setInterval(() => { if (state.authed) load(false).catch(console.error); }, 5000);
</script>
</body>
</html>`
