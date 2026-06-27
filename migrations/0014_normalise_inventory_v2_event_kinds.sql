UPDATE events
SET event_kind = replace(event_kind, '.event.v2', '.v2')
WHERE event_kind LIKE '%.event.v2';
