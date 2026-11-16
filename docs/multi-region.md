# Multi-region deployment

AgentFlow has no global tier. A multi-region deployment is one independent
`regional` cell per region, and the application in front decides which cell a
user belongs to. The cells share nothing at runtime: no cross-region database,
no read replicas, no cross-region session placement, no Qdrant sync.

That is deliberate. Each region is its own failure domain, the hot path never
pays a cross-region round trip, data residency is a property of the cell rather
than a policy layered on top, and there is no "global" write that can be
half-applied. The cost is that anything genuinely global — routing a person to
their region, a shared chat bot, billing — belongs to the front door.

## The cell

One cell per region:

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.eu-west.internal:5432/agentflow?sslmode=require
  admin:
    operators: { alice: ${ALICE_ADMIN_TOKEN} }
  shutdown_grace: 5s
media: { backend: s3, s3: { bucket: agentflow-eu-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-eu-files, region: eu-west-1 } }
memory:
  backends:
    hot: { provider: redis, config: { url: redis://redis.eu-west.internal:6379 } }
    vec: { provider: qdrant, config: { url: http://qdrant.eu-west.internal:6333 } }
```

Every instance in the cell runs this config with the same `runtime.region`.
Instances differ only in their listen addresses and their zone label. The
validator refuses a local blob directory, a SQLite memory backend, a log-plane
directory, a cluster store other than the runtime store, and a cell with no
shared admin token or named operators.

The cell's Redis is one region-wide cluster: sessions move between AZs within
the cell, so Redis locality is `region`. A durable conversation store must not
be on `volatile` (`instance`); validation refuses it unless the store sets
`locality_ok: true`.

## Pinning users to a region

The front door owns the user → region mapping. It should:

- create the profile in the user's home cell (`POST /v1/users/register` against
  that cell), and keep the mapping in its own database;
- send every subsequent request — `/v1/users/me`, channel traffic, Studio
  admin calls — to the same cell;
- route sign-up, invites, credential setup and password/identity changes to
  the home cell, never to a random one behind a load balancer.

`GET /v1/users/config` reports `"region"`, and `/admin/api/state` reports
`region`, `zone` and every live instance, so the front door (and an operator)
can tell which cell answered. A load balancer that spreads one user's requests
across cells will split their profile, memory and files; pin at the edge, not
by luck.

## A shared chat front door

A Telegram bot has exactly one receiver for its webhook — or one long-polling
consumer. Two cells configured with the same bot token will fight over updates,
and whichever wins handles users who belong to the other cell. The same is true
of most chat platforms.

The pattern is a bridge front door:

1. One service owns the bot token and receives every update.
2. It resolves the sender to a region (the same mapping as above).
3. It forwards the message to that cell's `bridge` channel.
4. The cell replies through the bridge, and the front door sends the reply.

The `bridge` channel is HMAC-signed, so the cell accepts traffic only from the
front door. Channels that are stateless per request (webhook, ghhook) can be
load-balanced to the user's cell directly; channels with a single external
receiver (Telegram, most chat SDKs) go through the front door.

## Admin and billing

Admin APIs are per cell. The console and Studio connect to one cell at a time;
Studio's connection profiles keep one entry per cell, so switching regions is
the account switcher in the rail footer rather than re-entering a token.

Billing fans out: call `/admin/api/usage` on every cell and sum the results.
Usage is deliberately not replicated across cells — a cross-region aggregation
would have to reconcile two ledgers, and the ledger is the thing quota
decisions are made from.

## Moving a user between regions

`GET /admin/api/users/{id}/bundle?public_key=<base64 X25519 public key>` returns
a zip with the profile, linked handles, memory rows in the user's scope, file
trees and blob bytes, group memberships, and credentials sealed to that public
key. `POST /admin/api/users/import` on the destination cell takes the zip and
the matching private key, recreates the profile with the same user uuid,
re-encrypts the credentials with that cell's master key, and reports what it
could not carry (a group the destination does not have, a store binding it does
not expose).

The CLI wraps both ends:

```
agentflow user keygen eu-move
agentflow user export u_abc -url https://eu-west.internal -pubkey eu-move.pub -out u_abc.zip
agentflow user import u_abc.zip -url https://us-east.internal -key eu-move.key
```

Move a user while they are idle: their sessions belong to the old cell until
the bundle is imported, and traffic sent to the new cell before that creates a
second, empty conversation. The front door should stop routing to the old cell,
drain (the old cell's `shutdown_grace` does not help here — this is a routing
change), import, then send traffic to the new cell.

## Disaster recovery

Each cell has its own PostgreSQL primary. A region outage is recovered by
standing up a replacement cell, not by promoting a global primary:

1. Keep a cross-region standby of the cell's PostgreSQL (streaming
   replication or a scheduled snapshot — the engine does not manage it).
2. Keep the S3 buckets replicated to the replacement region.
3. On loss of the region, promote the standby and start a cell with
   `runtime.region` set to the replacement region's label, pointing at the
   promoted database and the replicated buckets.
4. Re-point the front door's region mapping at the new cell. Sessions resume
   from the shared store; the `shutdown_grace` drain does not apply to a
   crashed cell, so peers in the old cell (if any survived) hand over at the
   session lease TTL.
5. Recreate the identity-provider and bot credentials in the new cell. They
   are environment/config, not data, and are the step most often missed.

A user move between cells is the same bundle mechanism at a smaller scale. A
whole-region move is a database promotion plus a bundle for any users whose
home cell must change.

## What not to do

- Do not point two cells at one PostgreSQL to "share" state: the lease table
  would arbitrate sessions across regions, which is the global tier this
  design deliberately does not have, and the cross-region latency would land
  on every lease renewal.
- Do not put a cross-region Redis or Qdrant in front of two cells. A region's
  Redis is its own; a shared one reintroduces the cross-region dependency.
- Do not use one Telegram bot token in two cells.
- Do not replicate the usage ledger and sum replicas; fan out reads instead.
