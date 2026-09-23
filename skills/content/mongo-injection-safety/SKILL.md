---
name: mongo-injection-safety
description: Quote user input in MongoDB $regex filters and prefer typed filter structs over bson.M from request bodies. Use whenever an LLM-generated or user-supplied string lands in a Mongo query.
category: security
priority: 6
---

# Mongo injection safety

MongoDB's query language is permissive: a string field can carry a
`$regex` value with arbitrary regex syntax, and a struct field can
carry operator documents (`{"$gt": ...}`, `{"$ne": null}`). Both are
NoSQL injection vectors when the input came from outside the
process.

## Rule 1: quote user input in $regex

`regexp.QuoteMeta` (Go) — or your language's equivalent — turns
`foo.*` into the literal string `foo\.\*`. Use it on every
user-controlled fragment before substituting into a `$regex` value.
The cost is one allocation; the benefit is that a malicious "search
term" can no longer turn into a CPU-burning catastrophic-backtracking
regex or match unintended documents.

```go
filter := bson.M{
    "name": bson.M{
        "$regex":   regexp.QuoteMeta(userInput),
        "$options": "i",
    },
}
```

If the user wants real regex support, accept regex *syntactically*
through a separate, narrowly-scoped endpoint and time-bound the
query.

## Rule 2: never bind request bodies straight into bson.M

```go
// BAD
var filter bson.M
json.Unmarshal(body, &filter)            // attacker controls operators
coll.Find(ctx, filter)
```

`bson.M` is a free-form map. Once the attacker controls the keys
they can inject `$where: "this..."`, `$ne: null` to bypass equality
checks, or `$expr: {...}` to evaluate arbitrary expressions.

Define a static request struct with only the fields the endpoint
needs, and build the `bson.M` inside the handler:

```go
type listReq struct {
    Name string `json:"name"`
    Limit int   `json:"limit"`
}
var req listReq
if err := c.BindJSON(&req); err != nil { return badRequest(err) }
filter := bson.M{"name": req.Name}      // operators impossible here
```

## Rule 3: cap and sort safely

- Always pass a `Limit` to `Find` so a wide query can't drain the
  cursor.
- Sort fields should be from a small allowlist; never accept a raw
  client-supplied `sort` doc.

## Anti-patterns

- Quoting only on "obvious" fields and forgetting `tags`,
  `description`, secondary search fields.
- Using regex when an exact-match would do — exact match has no
  injection surface.
- Per-request projection driven by client input (lets the client
  enumerate fields you didn't mean to expose).
