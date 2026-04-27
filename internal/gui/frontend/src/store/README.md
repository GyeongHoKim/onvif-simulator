# Frontend state management

This app uses **Zustand** for global state.

## Why Zustand

- Tiny footprint; no provider wiring (fits Wails' single-process model).
- Works cleanly with external event sources: we push records into the store
  from `runtime.EventsOn("event:new", ...)` handlers without needing a reducer
  action plumbing pass-through.
- Plays well with React 19 concurrent rendering (selectors are isolated).

## What lives in the store

- `status`: dashboard snapshot polled every 1s from `Status()`.
- `config`: deep-copied `config.Config` from `ConfigSnapshot()`; refreshed after
  any successful mutation.
- `users`: `UserView[]` from `Users()`; refreshed alongside auth mutations.
- `log`: ring buffer (bounded to 500) of `LogEntry` records fed by the
  `event:new` and `mutation:new` Wails events.

## What does NOT live in the store

- Form drafts (e.g. the Add-profile dialog fields). Keep those local with
  `useState`; commit to the store by calling the backend + refreshing the
  snapshot.
- Modal/dialog open state. Local `useState` per screen.
