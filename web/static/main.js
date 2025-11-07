(function(){
  const el = (html) => {
    const t = document.createElement('template');
    t.innerHTML = html.trim();
    return t.content.firstChild;
  };

  const LINES = ["outer-unified","inner-unified","outer-zf","inner-zf"];

  const cards = document.getElementById('cards');
  const charts = document.getElementById('charts');

  function numberInput(id, label, value, min, max, step) {
    return el(`<div>
      <label for="${id}">${label}</label>
      <input id="${id}" type="number" value="${value}" min="${min}" max="${max}" step="${step}"/>
    </div>`);
  }

  function textInput(id, label, value) {
    return el(`<div>
      <label for="${id}">${label}</label>
      <input id="${id}" type="text" value="${value}"/>
    </div>`);
  }

  function checkbox(id, label, checked) {
    return el(`<div style="display:flex; align-items:center; gap:6px; margin-top:8px;">
      <input id="${id}" type="checkbox" ${checked ? 'checked' : ''}/>
      <label for="${id}" style="margin:0">${label}</label>
    </div>`);
  }

  function fmtPct(x) { return (x*100).toFixed(1) + "%"; }

  function createCard(lineId, cfg) {
    const card = el(`<div class="card" id="card-${lineId}"></div>`);
    card.appendChild(el(`<h3>${cfg.name}</h3>`));

    const row1 = el(`<div class="row"></div>`);
    row1.appendChild(numberInput(`base-${lineId}`, "基础延迟 (ms)", cfg.base_latency_ms, 0, 5000, 10));
    row1.appendChild(numberInput(`jitter-${lineId}`, "抖动 (±ms)", cfg.jitter_ms, 0, 5000, 10));

    const row2 = el(`<div class="row"></div>`);
    row2.appendChild(numberInput(`erate-${lineId}`, "错误率 (0..1)", cfg.error_rate, 0, 1, 0.01));
    row2.appendChild(numberInput(`trate-${lineId}`, "超时率 (0..1)", cfg.timeout_rate, 0, 1, 0.01));

    const row3 = el(`<div class="row"></div>`);
    row3.appendChild(numberInput(`tms-${lineId}`, "超时时长 (ms)", cfg.timeout_ms, 0, 120000, 100));
    row3.appendChild(textInput(`nbw-${lineId}-start`, "夜间开始 HH:MM", cfg.night_block_window.start));

    const row4 = el(`<div class="row"></div>`);
    row4.appendChild(textInput(`nbw-${lineId}-end`, "夜间结束 HH:MM", cfg.night_block_window.end));
    row4.appendChild(el('<div></div>'));

    const cb1 = checkbox(`enabled-${lineId}`, "启用该线路", cfg.enabled);
    const cb2 = checkbox(`nb-${lineId}`, "夜间自动屏蔽", cfg.night_block_enabled);

    const btnBar = el(`<div style="margin-top:10px; display:flex; gap:8px; align-items:center;">
      <button id="save-${lineId}">保存配置</button>
      <span class="small" id="hint-${lineId}"></span>
    </div>`);

    card.appendChild(row1); card.appendChild(row2); card.appendChild(row3); card.appendChild(row4);
    card.appendChild(cb1); card.appendChild(cb2); card.appendChild(btnBar);

    btnBar.querySelector(`#save-${lineId}`).addEventListener('click', async () => {
      const body = {
        name: cfg.name,
        enabled: document.getElementById(`enabled-${lineId}`).checked,
        base_latency_ms: parseInt(document.getElementById(`base-${lineId}`).value,10),
        jitter_ms: parseInt(document.getElementById(`jitter-${lineId}`).value,10),
        error_rate: parseFloat(document.getElementById(`erate-${lineId}`).value),
        timeout_rate: parseFloat(document.getElementById(`trate-${lineId}`).value),
        timeout_ms: parseInt(document.getElementById(`tms-${lineId}`).value,10),
        night_block_enabled: document.getElementById(`nb-${lineId}`).checked,
        night_block_window: {
          start: document.getElementById(`nbw-${lineId}-start`).value,
          end: document.getElementById(`nbw-${lineId}-end`).value,
        }
      };
      const res = await fetch(`/admin/line/${lineId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      if (res.ok) {
        document.getElementById(`hint-${lineId}`).textContent = "已保存 ✓";
        setTimeout(()=> document.getElementById(`hint-${lineId}`).textContent = "", 1200);
      } else {
        document.getElementById(`hint-${lineId}`).textContent = "保存失败";
      }
    });

    return card;
  }

  async function loadConfig() {
    const res = await fetch('/admin/config');
    const cfg = await res.json();
    return cfg.lines;
  }

  function createChart(lineId) {
    const wrap = el(`<div style="margin-top: 12px;"></div>`);
    wrap.appendChild(el(`<div class="small" style="margin-bottom:6px;">${lineId} RPS / Avg Latency</div>`));
    const canvas = el(`<canvas id="c-${lineId}" width="800" height="160"></canvas>`);
    wrap.appendChild(canvas);
    charts.appendChild(wrap);

    const ctx = canvas.getContext('2d');

    function draw(secs, rps, avg) {
      const W = canvas.width, H = canvas.height;
      ctx.clearRect(0,0,W,H);
      // axes
      ctx.globalAlpha = 1;
      ctx.beginPath();
      ctx.moveTo(30,10); ctx.lineTo(30,H-20); ctx.lineTo(W-10,H-20); ctx.stroke();

      // Determine scales
      const maxR = Math.max(5, ...rps);
      const maxL = Math.max(50, ...avg);

      // Draw RPS line (left axis)
      const plot = (arr, color, scale, yOffset) => {
        const n = arr.length;
        if (n === 0) return;
        const usableW = W-50;
        ctx.beginPath();
        for (let i=0;i<n;i++) {
          const x = 30 + i*(usableW/Math.max(1,n-1));
          const y = (H-20) - (arr[i]/scale)*(H-40);
          if (i===0) ctx.moveTo(x,y); else ctx.lineTo(x,y);
        }
        ctx.strokeStyle = color;
        ctx.stroke();
      };
      plot(rps, "#7dd3fc", maxR, 0);

      // Latency line (right axis), normalize to same scale for simplicity
      plot(avg, "#86efac", Math.max(maxR, maxL), 0);

      // legends
      ctx.fillStyle = "#9ca3af";
      ctx.font = "12px system-ui, sans-serif";
      ctx.fillText(`max RPS=${maxR}`, 40, 16);
      ctx.fillText(`max Avg(ms)=${Math.max(...avg)}`, 140, 16);
    }

    return { draw };
  }

  async function init() {
    const lines = await loadConfig();

    // cards
    LINES.forEach(id => {
      cards.appendChild(createCard(id, lines[id]));
    });

    // charts
    const chartMap = {};
    LINES.forEach(id => { chartMap[id] = createChart(id); });

    async function tick() {
      const res = await fetch('/metrics/snapshot');
      const snap = await res.json();
      LINES.forEach(id => {
        const s = snap.series[id];
        chartMap[id].draw(s.sec, s.rps, s.latency_avg);
      });
      // also update small KPI table in cards header
      setTimeout(tick, 1000);
    }
    tick();

    document.getElementById('resetBtn').addEventListener('click', async () => {
      await fetch('/admin/reset', { method: 'POST' });
    });
  }

  init();
})();