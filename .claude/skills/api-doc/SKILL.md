---
name: api-doc
description: Update docs/openapi.yaml when adding or modifying API endpoints
disable-model-invocation: true
---

# Update API Documentation

Keep `docs/openapi.yaml` (OpenAPI 3.0.3) in sync when adding or modifying API endpoints.

## Arguments

- First argument: description of the API change (e.g., "added GET /api/v1/works endpoint")

## Steps

1. **Read the current OpenAPI spec:**
   ```
   Read docs/openapi.yaml
   ```

2. **Identify the endpoint in Go source** — routes are registered in `internal/server/server.go` using Gin:
   ```go
   v1.GET("/endpoint", s.handlerFunc)
   ```

3. **Read the handler function** to understand request/response types

4. **Update docs/openapi.yaml** with:
   - Path definition under `paths:`
   - Request body schema (if POST/PUT/PATCH)
   - Response schemas (200, 400, 404, 500)
   - Proper tag assignment (match existing tags)
   - Parameter definitions (path params, query params)

5. **Bump the version** in the openapi.yaml header and `info.version`

## Template for a new endpoint

```yaml
  /endpoint:
    get:
      tags:
        - TagName
      summary: Short description
      description: Longer description
      operationId: operationName
      parameters:
        - $ref: '#/components/parameters/idPath'
      responses:
        '200':
          description: Success
          content:
            application/json:
              schema:
                type: object
                properties:
                  field:
                    type: string
        '404':
          description: Not found
      security:
        - bearerAuth: []
```

## Rules

- Follow OpenAPI 3.0.3 spec
- Use existing `$ref` components where possible (parameters, schemas)
- All endpoints require `security: [bearerAuth: []]` except Health and Events
- Use existing tags — only add new tags if truly a new domain
- Keep response schemas consistent with actual Go struct JSON output
