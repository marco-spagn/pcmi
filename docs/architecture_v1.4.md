┌─────────────┐
│   API       │
│  (Fiber)    │
└──────┬──────┘
       │
   Tenant Middleware (RBAC + Context)
       │
┌──────▼──────┐
│ PostgreSQL  │ ← Row Level Security (RLS)
│ + pgvector  │
└─────────────┘
       │
   Tenant-scoped Redis channels