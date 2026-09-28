# Response Package (`pkg/response`)

The `response` package standardizes all JSON API responses, pagination envelopes, and error codes across all platform services.

---

## 1. Response Envelopes

### Success Envelope
```json
{
  "success": true,
  "data": { ... },
  "meta": { "page": 1, "per_page": 20, "total_items": 100, "total_pages": 5 }
}
```

### Error Envelope
```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "Resource with specified ID was not found",
    "details": null
  }
}
```

---

## 2. Functions & API Reference

### Success Responses
* `response.JSON(w http.ResponseWriter, status int, data T)`  
  Sends a JSON response with the given HTTP status code and payload wrapped in `APIResponse[T]`.
* `response.JSONWithMeta(w http.ResponseWriter, status int, data T, meta *Pagination)`  
  Sends a paginated JSON response with page, per_page, total_items, and total_pages.

### Error Responses
* `response.BadRequest(w http.ResponseWriter, message string, details any)` (`400 Bad Request`)
* `response.Unauthorized(w http.ResponseWriter, message string)` (`401 Unauthorized`)
* `response.Forbidden(w http.ResponseWriter, message string)` (`403 Forbidden`)
* `response.NotFound(w http.ResponseWriter, message string)` (`404 Not Found`)
* `response.Conflict(w http.ResponseWriter, message string, details any)` (`409 Conflict`)
* `response.InternalServerError(w http.ResponseWriter, message string)` (`500 Internal Server Error`)
* `response.Error(w http.ResponseWriter, status int, code, message string, details any)` (Custom status & code)

---

## 3. How to Test

```bash
go test -v ./pkg/response
```
