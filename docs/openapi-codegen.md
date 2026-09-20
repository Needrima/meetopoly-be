# OpenAPI → mobile client

Meetopoly HTTP contract: [`api/openapi.yaml`](../api/openapi.yaml).

Mobile generates TypeScript with **[orval](https://orval.dev/)** (MIT):

```bash
cd meetopoly-mobile
npm run api:generate
```

Output: `meetopoly-mobile/api/generated/` (endpoints + models).  
Public imports: `@/api/services`, `@/api/types`.  
HTTP mutator: `meetopoly-mobile/api/client.ts` (`apiMutator`).

After changing the OpenAPI file, regenerate before implementing FE/BE handlers that depend on new fields.
