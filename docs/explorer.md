# Explore local traces

Run `tracelet ui` from the initialized project, or `tracelet ui --no-open` to copy its private local URL. The browser reads the real local API. It uses no external fonts, scripts or hosted service. A stale link asks you to reconnect using the current session's URL.

The overview shows capture/history mode, receipt/rejection information and physical storage use. Search matches a literal operation-name substring in any span. Sort by newest, slowest or errors. Pages are bounded and refresh is manual. A revision change restarts paging instead of silently skipping or duplicating traces. Settings are read-only; edit the project configuration and restart to change them.

Select a trace to load its complete timeline metadata for one stored revision. Node pages contain at most 500 spans. Loading restarts once if new data changes the revision; continuing changes request a manual retry. The UI refuses timelines above 100,000 nodes explicitly rather than displaying an apparently complete subset. All retained data remains in storage.

The waterfall preserves observed hierarchy and uses elapsed offsets calculated from exact nanoseconds. Overlapping child durations are not added together. Roots, orphans, cycles and time-skew warnings remain visible. No root or quiet period proves a trace complete. Only rows in the visible scroll area are drawn, so large traces stay usable.

- Arrow Up/Down move between visible spans; Home/End move to the first/last.
- Enter selects a span. Left collapses a branch or moves to its parent; Right expands or moves to its first child.
- Collapsing a branch containing the selected span selects its parent. The selection remains stable while scrolling and zooming.
- Fit trace resets the timeline scale. Zoom increases the scale; use horizontal scrolling to reach later time ranges. Narrow screens keep wide trace tables/timelines inside their own scroll areas.
- Selected summaries show IDs, parent, service, status, exact UTC start and duration. Full attribute/event inspection is added by T15.

T14 was checked in the packaged binary with a real Go sample and a 2,001-span synthetic trace, including keyboard navigation to the last span, collapse recovery, zoom and 390px/1280px layouts. Only 22 table rows were rendered for the large trace. The formal timing benchmark remains T21.
