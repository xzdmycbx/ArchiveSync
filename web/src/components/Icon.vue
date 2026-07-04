<script setup>
// A single self-contained SVG icon system. No emoji, no external assets.
// Line icons share a 24px grid, 1.75 stroke, round caps/joins; brand glyphs
// (discord/telegram) are filled paths.
defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 20 },
  stroke: { type: [Number, String], default: 1.75 },
})

const paths = {
  dashboard: '<rect x="3" y="3" width="7" height="9" rx="1.6"/><rect x="14" y="3" width="7" height="5" rx="1.6"/><rect x="14" y="12" width="7" height="9" rx="1.6"/><rect x="3" y="16" width="7" height="5" rx="1.6"/>',
  target: '<circle cx="12" cy="12" r="8.4"/><circle cx="12" cy="12" r="4.4"/><circle cx="12" cy="12" r="0.7" fill="currentColor" stroke="none"/>',
  cloud: '<path d="M17.5 18.5H7a4 4 0 0 1-.8-7.92 5.5 5.5 0 0 1 10.6-1.36A3.75 3.75 0 0 1 17.5 18.5Z"/>',
  bell: '<path d="M18 8.5a6 6 0 1 0-12 0c0 6-2.4 7.5-2.4 7.5h16.8S18 14.5 18 8.5Z"/><path d="M10.2 20a2.1 2.1 0 0 0 3.6 0"/>',
  history: '<path d="M3.5 9a9 9 0 1 1-.4 4.5"/><path d="M3 3.5v5h5"/><path d="M12 8.2v4l2.6 1.6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  edit: '<path d="M4 20.5h4L18.6 9.9a2.1 2.1 0 0 0-3-3L5 17.5v3Z"/><path d="M13.7 6.7l3 3"/>',
  trash: '<path d="M4 7h16"/><path d="M9 7V5.2A1.2 1.2 0 0 1 10.2 4h3.6A1.2 1.2 0 0 1 15 5.2V7"/><path d="M6.2 7l.8 12a2 2 0 0 0 2 1.9h6a2 2 0 0 0 2-1.9L18 7"/><path d="M10 11v6M14 11v6"/>',
  play: '<path d="M7.5 5.6v12.8a1 1 0 0 0 1.5.86l10.6-6.4a1 1 0 0 0 0-1.72L9 4.74a1 1 0 0 0-1.5.86Z"/>',
  activity: '<path d="M3 12h3.5l2.5 7 4-15 2.5 8H21"/>',
  refresh: '<path d="M20.5 11A8.5 8.5 0 0 0 6.2 6L3 9"/><path d="M3 4v5h5"/><path d="M3.5 13A8.5 8.5 0 0 0 17.8 18L21 15"/><path d="M21 20v-5h-5"/>',
  close: '<path d="M6 6l12 12M18 6 6 18"/>',
  check: '<path d="M5 12.5 10 17.5 19.5 6.5"/>',
  checkCircle: '<circle cx="12" cy="12" r="9"/><path d="M8.4 12.3 11 15l4.7-5.8"/>',
  xCircle: '<circle cx="12" cy="12" r="9"/><path d="M9 9l6 6M15 9l-6 6"/>',
  alert: '<path d="M12 4 2.9 19.5a1.1 1.1 0 0 0 .95 1.6h16.3a1.1 1.1 0 0 0 .95-1.6L12 4Z"/><path d="M12 10.5v4"/><circle cx="12" cy="17.6" r="0.5" fill="currentColor" stroke="none"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7.2v5l3.4 2"/>',
  chevronDown: '<path d="M6 9.5l6 6 6-6"/>',
  chevronRight: '<path d="M9.5 6l6 6-6 6"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><circle cx="12" cy="8" r="0.65" fill="currentColor" stroke="none"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="M20 20l-3.6-3.6"/>',
  logout: '<path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4"/><path d="M9.5 8 5.5 12l4 4"/><path d="M5.5 12H16"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2.2M12 19.3v2.2M4.7 4.7l1.6 1.6M17.7 17.7l1.6 1.6M2.5 12h2.2M19.3 12h2.2M4.7 19.3l1.6-1.6M17.7 6.3l1.6-1.6"/>',
  moon: '<path d="M20.5 14.5A8.5 8.5 0 0 1 9.5 3.5 8.5 8.5 0 1 0 20.5 14.5Z"/>',
  folder: '<path d="M3 7.2a2 2 0 0 1 2-2h3.7l2 2.4H19a2 2 0 0 1 2 2V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7.2Z"/>',
  database: '<ellipse cx="12" cy="6" rx="7" ry="3"/><path d="M5 6v6c0 1.66 3.13 3 7 3s7-1.34 7-3V6"/><path d="M5 12v6c0 1.66 3.13 3 7 3s7-1.34 7-3v-6"/>',
  mail: '<rect x="3" y="5" width="18" height="14" rx="2.4"/><path d="M4 7.5l7.3 5.2a1.2 1.2 0 0 0 1.4 0L20 7.5"/>',
  webhook: '<path d="M14.5 14.2 16 11.6"/><path d="M8.6 10 7.2 12.4a3.6 3.6 0 1 0 5 1.2"/><path d="M13 6.5a3.6 3.6 0 1 0-4.7 3.4"/><path d="M12.7 16.4h3a3.6 3.6 0 1 0-3-5.4"/>',
  shield: '<path d="M12 3 5 6v5c0 4.5 3 7.9 7 9 4-1.1 7-4.5 7-9V6l-7-3Z"/><path d="M9 11.8 11 14l4-4.6"/>',
  calendar: '<rect x="3.5" y="5" width="17" height="15.5" rx="2.4"/><path d="M3.5 9.5h17M8 3v4M16 3v4"/>',
  layers: '<path d="M12 3 3 7.5l9 4.5 9-4.5L12 3Z"/><path d="M3 12l9 4.5 9-4.5"/><path d="M3 16.5 12 21l9-4.5"/>',
  zap: '<path d="M13 3 4.5 13.5H10l-1 7.5 8.5-10.5H12l1-7.5Z"/>',
  server: '<rect x="3" y="4" width="18" height="7" rx="2.2"/><rect x="3" y="13" width="18" height="7" rx="2.2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  archive: '<path d="M4 8.5 6 4.5h12l2 4"/><path d="M4 8.5V18a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8.5H4Z"/><path d="M9.5 12.5h5"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2.2"/><path d="M5 15H4.2A2.2 2.2 0 0 1 2 12.8V4.2A2.2 2.2 0 0 1 4.2 2h8.6A2.2 2.2 0 0 1 15 4.2V5"/>',
  slash: '<circle cx="12" cy="12" r="9"/><path d="M5.6 5.6l12.8 12.8"/>',
  eye: '<path d="M2.2 12S6 5.5 12 5.5 21.8 12 21.8 12 18 18.5 12 18.5 2.2 12 2.2 12Z"/><circle cx="12" cy="12" r="3.2"/>',
  download: '<path d="M12 3.5v11"/><path d="M7.5 10 12 14.5 16.5 10"/><path d="M4.5 20h15"/>',
  file: '<path d="M13 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V9l-6-6Z"/><path d="M13 3v6h6"/>',
  folderOpen: '<path d="M3 7.4A1.4 1.4 0 0 1 4.4 6H9l2 2.3h6.6A1.4 1.4 0 0 1 19 9.7v.5"/><path d="M2.7 11h18.6l-1.66 7.3A1.6 1.6 0 0 1 18.08 19.6H5.92a1.6 1.6 0 0 1-1.56-1.3L2.7 11Z"/>',
  home: '<path d="M4 11.5 12 4l8 7.5"/><path d="M6 10.2V19a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-8.8"/>',
  hardDrive: '<rect x="3" y="13" width="18" height="7" rx="2"/><path d="M6.2 5.5 4.3 12.6a1 1 0 0 0 .02.4h15.36a1 1 0 0 0 .02-.4L17.8 5.5A2 2 0 0 0 15.87 4H8.13A2 2 0 0 0 6.2 5.5Z"/><path d="M7 16.5h.01M11 16.5h.01"/>',
  chevronLeft: '<path d="M15 6l-6 6 6 6"/>',
  back: '<path d="M19 12H5.5"/><path d="M11.5 6 5.5 12l6 6"/>',
  external: '<path d="M14 5h5v5"/><path d="M19 5 11 13"/><path d="M18.5 14v4a2 2 0 0 1-2 2h-10a2 2 0 0 1-2-2v-10a2 2 0 0 1 2-2h4"/>',
  discord: '<path fill="currentColor" stroke="none" d="M19.6 5.6A16 16 0 0 0 15.5 4.3l-.25.5A14.8 14.8 0 0 1 12 4.45c-1.12 0-2.24.12-3.25.35l-.25-.5A16 16 0 0 0 4.4 5.6C1.9 9.3 1.2 12.9 1.55 16.45A16.1 16.1 0 0 0 6.5 19l.62-1.06c-.6-.22-1.16-.5-1.68-.82l.41-.31a10.9 10.9 0 0 0 9.3 0l.41.31c-.52.32-1.08.6-1.68.82L14.5 19a16 16 0 0 0 4.95-2.55c.42-4.15-.6-7.72-2.6-10.85ZM8.9 14.3c-.96 0-1.75-.9-1.75-2s.77-2 1.75-2 1.77.9 1.75 2c0 1.1-.79 2-1.75 2Zm6.2 0c-.96 0-1.75-.9-1.75-2s.77-2 1.75-2 1.77.9 1.75 2c0 1.1-.78 2-1.75 2Z"/>',
  telegram: '<path fill="currentColor" stroke="none" d="M21.9 4.4 2.9 11.2c-.95.35-.94 1.7.03 2.02l4.6 1.5 1.78 5.35c.24.72 1.16.9 1.66.32l2.5-2.9 4.45 3.28c.6.44 1.45.1 1.63-.63L23.4 5.6c.22-.9-.66-1.55-1.5-1.2ZM9.9 14.7l8.1-5.1-6.4 6.1-.2 3.4-1.5-4.4Z"/>',
}
</script>

<template>
  <svg
    class="ico"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    :stroke-width="stroke"
    stroke="currentColor"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    v-html="paths[name] || ''"
  />
</template>

<style scoped>
.ico { display: block; flex: 0 0 auto; }
</style>
