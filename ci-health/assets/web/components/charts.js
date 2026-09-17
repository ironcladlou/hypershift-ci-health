import { useState } from "preact/hooks";
import { html, rateClass } from "../ui.js";
import { Tooltip } from "./tooltip.js";

function slotDetails(slot) {
  if (!slot || slot[0] === 0) return "no runs";
  if (slot[3] > 0) return `${slot[0]} runs · ${slot[1]} pass · ${slot[2]} fail · ${slot[3]} infra`;
  return `${slot[0]} runs · ${slot[1]} pass`;
}

export function Sparkline({ data, slots, correlated = [], showDates = true, dateEvery = 1 }) {
  const [tooltip, setTooltip] = useState(null);
  if (!data?.length || !slots?.length) return null;
  correlated = correlated || [];
  const values = slots.map((key, index) => {
    const time = new Date(key.replace(" ", "T") + "Z");
    return { slot: data[index], time, key };
  });
  const boundaries = values.map((entry, index) => index > 0 &&
    entry.time.getUTCDate() !== values[index - 1].time.getUTCDate());
  const dayBounds = boundaries.filter(Boolean).length;
  const gap = 1;
  const dayGap = 4;
  const width = 300;
  const markerHeight = correlated.length ? 6 : 0;
  const barHeight = 20;
  const labelHeight = showDates && dayBounds ? 12 : 0;
  const totalGap = (values.length - 1 - dayBounds) * gap + dayBounds * dayGap;
  const slotWidth = (width - totalGap) / values.length;
  const correlatedSet = new Set(correlated);
  let x = 0;
  let boundaryIndex = 0;
  const bars = [];
  const labels = [];

  values.forEach((entry, index) => {
    if (index > 0) {
      if (boundaries[index]) {
        if (boundaryIndex % dateEvery === 0) {
          labels.push({ x: x + dayGap / 2, text: `${entry.time.getUTCMonth() + 1}/${entry.time.getUTCDate()}` });
        }
        boundaryIndex++;
        x += dayGap;
      } else {
        x += gap;
      }
    }
    const rate = entry.slot?.[0] ? 100 * entry.slot[1] / entry.slot[0] : 0;
    const fill = !entry.slot?.[0] ? "var(--gridline)" :
      rate >= 80 ? "var(--good)" : rate >= 50 ? "var(--warning)" : "var(--critical)";
    bars.push({ x, fill, title: slotDetails(entry.slot), correlated: correlatedSet.has(index) });
    x += slotWidth;
  });

  const height = markerHeight + barHeight + labelHeight;
  const showTooltip = (event, text) => {
    const host = event.currentTarget.closest(".graph-tooltip-host").getBoundingClientRect();
    const target = event.currentTarget.getBoundingClientRect();
    setTooltip({ text, left: target.left + target.width / 2 - host.left, top: target.top - host.top });
  };
  return html`<span class="graph-tooltip-host" onMouseLeave=${() => setTooltip(null)}>
    <svg class="sparkline" width=${width} height=${height} viewBox=${`0 0 ${width} ${height}`} role="img" aria-label="Pass-rate history">
      ${bars.map((bar, index) => html`<g key=${index}>
        ${bar.correlated && html`<rect x=${bar.x} y="0" width=${slotWidth} height="4" rx="1" fill="var(--serious)"
          onMouseEnter=${event => showTooltip(event, "Periodic also failing — likely infrastructure")}><title>Periodic also failing — likely infrastructure</title></rect>`}
        <rect class="graph-hit" tabindex="0" x=${bar.x} y=${markerHeight} width=${slotWidth} height=${barHeight} rx="2" fill=${bar.fill}
          onMouseEnter=${event => showTooltip(event, bar.title)} onFocus=${event => showTooltip(event, bar.title)} onBlur=${() => setTooltip(null)}><title>${bar.title}</title></rect>
      </g>`)}
      ${showDates && labels.map((item, index) => html`<text key=${index} x=${item.x} y=${height - 1} text-anchor="middle">${item.text}</text>`)}
    </svg>
    ${tooltip && html`<span class="graph-tip" role="tooltip" style=${{ left: `${tooltip.left}px`, top: `${tooltip.top}px` }}>${tooltip.text}</span>`}
  </span>`;
}

export function RateChart({ data, slots, color = "pre" }) {
  const [tooltip, setTooltip] = useState(null);
  if (!data?.length || !slots?.length) return null;
  const rates = data.map(slot => slot?.[0] ? 100 * slot[1] / slot[0] : null);
  const points = rates.map((rate, index) => rate == null ? null : {
    x: rates.length === 1 ? 70 : 2 + index * 136 / (rates.length - 1),
    y: 26 - rate * 0.24,
    rate,
    label: slots[index],
  }).filter(Boolean);
  if (!points.length) return html`<svg class="rate-chart" width="140" height="28" aria-hidden="true"></svg>`;
  const stroke = color === "per" ? "var(--series-2)" : "var(--series-1)";
  const step = rates.length > 1 ? 136 / (rates.length - 1) : 136;
  const showTooltip = (event, point) => {
    const host = event.currentTarget.closest(".graph-tooltip-host").getBoundingClientRect();
    const target = event.currentTarget.getBoundingClientRect();
    setTooltip({ text: `${point.label}: ${point.rate.toFixed(1)}% pass`, left: target.left + target.width / 2 - host.left, top: 0 });
  };
  return html`<span class="graph-tooltip-host" onMouseLeave=${() => setTooltip(null)}>
    <svg class="rate-chart" width="140" height="28" viewBox="0 0 140 28" role="img" aria-label="Pass-rate trend">
      <polygon class="rate-area" points=${`${points[0].x},26 ${points.map(point => `${point.x},${point.y}`).join(" ")} ${points[points.length - 1].x},26`}
        fill=${stroke} fill-opacity="0.08" />
      <polyline points=${points.map(point => `${point.x},${point.y}`).join(" ")} fill="none" stroke=${stroke} stroke-width="1.5" vector-effect="non-scaling-stroke" />
      ${points.map((point, index) => html`<rect class="graph-hit" key=${index} tabindex="0" x=${Math.max(0, point.x - step / 2)} y="0"
        width=${Math.min(step, 140)} height="28" fill="transparent" onMouseEnter=${event => showTooltip(event, point)}
        onFocus=${event => showTooltip(event, point)} onBlur=${() => setTooltip(null)}><title>${point.label}: ${point.rate.toFixed(1)}% pass</title></rect>`)}
    </svg>
    ${tooltip && html`<span class="graph-tip" role="tooltip" style=${{ left: `${tooltip.left}px`, top: `${tooltip.top}px` }}>${tooltip.text}</span>`}
  </span>`;
}

export function Trend({ value }) {
  if (value == null) return html`<span class="trend-flat">—</span>`;
  const displayed = value.toFixed(1);
  if (value > 1) return html`<span class="trend-up">▲ +${displayed}</span>`;
  if (value < -1) return html`<span class="trend-down">▼ ${displayed}</span>`;
  return html`<span class="trend-flat">— ${displayed}</span>`;
}

export function RateSummary({ job }) {
  const rate = job.rate ?? 0;
  const runs = job.runs ?? 0;
  if (!runs) return html`<span class="rate-na">—</span>`;
  const previous = job.prev_runs > 0 ? `previous ${Number(job.prev || 0).toFixed(1)}%` : "no previous data";
  const detail = job.infra_fails > 0
    ? `${rate.toFixed(1)}% pass · ${runs} runs · ${job.test_fails || 0} test fail · ${job.infra_fails} infra · ${previous}`
    : `${rate.toFixed(1)}% pass · ${runs} runs · ${job.fails || 0} fails · ${previous}`;
  return html`<${Tooltip} content=${detail} className="rate-summary">
    <span class=${`pass-rate ${rateClass(rate, runs)}`}>${rate.toFixed(0)}%</span> <${Trend} value=${job.trend} />
  <//>`;
}
