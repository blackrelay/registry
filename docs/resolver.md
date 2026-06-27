# Resolver

The resolver translates public game IDs and type IDs into semantic records with confidence and source IDs.

It currently resolves:
- character ID or name
- system ID or name
- enemy type ID

The main user-visible path is semantic killmail rendering. An NPC killmail with killer type `92096` resolves to:
```json
{
  "entityType": "enemy",
  "entityId": "enemy:stillness:type:92096",
  "displayName": "Caird [NPC]",
  "typeId": "92096",
  "confidence": "probable"
}
```

Unresolved participants are returned as explicit `unknown` values with warnings, so clients can render gaps without dropping actors.

When a killmail payload has no `killer_type_id`, the semantic killmail service treats `killer_id` as the public tenant/item identity exposed by the chain payload. NPC killer labelling requires an explicit `killer_type_id` or a sourced enemy killer relation. Without that evidence, the resolver returns a character lookup or an explicit unresolved actor with a warning.
