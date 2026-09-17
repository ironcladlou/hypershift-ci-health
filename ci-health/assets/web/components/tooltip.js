import { html } from "../ui.js";

export function Tooltip({ content, children, className = "" }) {
  if (!content) return children;
  return html`<span class=${`has-tip ${className}`} tabindex="0">
    <span class="tip" role="tooltip">${content}</span>${children}
  </span>`;
}
