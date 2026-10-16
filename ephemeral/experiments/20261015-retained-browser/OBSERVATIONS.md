# Whole-application retained-path browser observation

Fixture: `http://127.0.0.1:56511/orders`, started by the retained-qualification experiment. The fixture log reports `mode=whole-retained wholeApplication=true HostView=true`, native PID 60931, 8 application roots, 8 typed roots, a 265-package graph, and 4 affected packages (`ephemeral/experiments/20261015-retained-qualification/browser-run.log`). The browser was controlled through CUA only.

I opened the real hydrated page, entered `RETAINED_DRAFT_42`, and clicked Increment once. The initial browser showed total 3000, count 1, Hydrated true, notices 0, and document ID `ddda1c4b-9bf1-4f98-9f20-83d1cf7a66f7`. Actual source saves changed the native total to 4101, 5202, and 6303; each automatically updated the page/remotes, advanced notices to 1, 2, and 3, and preserved the draft, count, hydration, and document ID. The same native PID remained in use.

An invalid body edit was rejected. The browser kept 6303 and its state, showed `GO_COMPILE_REJECTED`, and advanced notices to 4. Repair to 7407 automatically updated the browser while preserving draft, count, hydration, and document ID; notices reached 5.

Captured timings in the fixture log are source-observation-to-install. Revision 1 (cold) prepared in 793.424 ms and installed in 3.699 ms. Warm revisions 2 and 3 prepared in 176.658 and 165.195 ms, installing in 3.861 and 3.714 ms. Repair revision 5 prepared in 161.491 ms and installed in 3.761 ms. These are not save-to-paint measurements. Revision 2/3 stages show 38.555–48.750 ms analysis, 32.423–33.436 ms native compile, and 87.994–90.286 ms artifact identity. This browser run demonstrates whole-application retained admission and working native updates, but the measured warm preparation is still above 137 ms.

## Browser evidence

The final CUA accessibility snapshot was:

```text
0 AXWebArea 127.0.0.1:56511/orders
  1 container
    2 heading Order total 7407, Value: 1
      3 text Order total 7407
    4 text field (settable) Value: RETAINED_DRAFT_42, ID: draft
    5 button Increment
    6 text Count 1 Hydrated true Notices 5 Ready 7407 Document ddda1c4b-9bf1-4f98-9f20-83d1cf7a66f7 Again 7407; initialized 1; quantity 2 Deferred 7407
```

A final screenshot was captured and visually inspected through CUA. It showed the same final values and an ordinary rendered page. The CUA screenshot result did not expose a local file destination. An attempt to use a `data:` URL to persist it was explicitly blocked by browser policy, so no screenshot file is claimed or archived here. The final AX snapshot and fixture's stage/rejection log are preserved above and in the original fixture log.

Scope: one real browser and one application instance. No second-instance isolation claim is made from this fixture. The page was rendered successfully and visibly; this lane did not use a control endpoint to substitute for native compilation.
