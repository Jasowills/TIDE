// Lighthouse CI rule set for the TIDE console (single-page app).
//
// What is gated and why:
// - largest-contentful-paint ≤ 2500ms — Good threshold; the console is a
//   debugging tool, slow paint directly costs incident-response time.
// - cumulative-layout-shift ≤ 0.1 — Good threshold; shifting controls under
//   a responder's cursor cause misclicks.
// - total-blocking-time ≤ 200ms — lab proxy for INP. INP itself is
//   FIELD-ONLY and can never appear here; asserting it would gate a number
//   Lighthouse never measures. Real INP comes from CrUX/RUM, not this file.
// - categories:performance ≥ 0.9, interactive ≤ 5s — overall guardrails.
// - resource budgets (warn): script ≤ 300KB, total ≤ 1MB — early warning
//   before a dependency bloats the bundle past the error gates.
// - FID is never asserted (deprecated, removed from web-vitals v5+).
// Uploads go to the filesystem (CI artifact) — never to an external
// temporary-storage endpoint that can fail the run on network alone.
module.exports = {
  ci: {
    collect: {
      url: ['http://localhost:4173/'],
      startServerCommand: 'npm run preview -- --port 4173 --strictPort',
      startServerReadyPattern: 'Local:',
      numberOfRuns: 3,
      settings: { preset: 'desktop' },
    },
    assert: {
      assertions: {
        'largest-contentful-paint': ['error', { maxNumericValue: 2500 }],
        'cumulative-layout-shift': ['error', { maxNumericValue: 0.1 }],
        'total-blocking-time': ['error', { maxNumericValue: 200 }],
        'categories:performance': ['error', { minScore: 0.9 }],
        interactive: ['error', { maxNumericValue: 5000 }],
        'resource-summary:script:size': ['warn', { maxNumericValue: 300000 }],
        'resource-summary:total:size': ['warn', { maxNumericValue: 1000000 }],
      },
    },
    upload: { target: 'filesystem', outputDir: './lhci-reports' },
  },
};
