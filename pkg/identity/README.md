# Identity Package (`pkg/identity`)

The `identity` package provides cryptographic and authentication primitives: bcrypt password hashing and HMAC-SHA256 JWT token generation and validation.

---

## 1. Features

* **Bcrypt Password Hashing:** Cost factor 12 prevents rainbow table and brute-force attacks.
* **Stateless JWT Tokens:** Signs and verifies HS256 tokens carrying `user_id` and `email` with expiration validation.

---

## 2. Functions & API Reference

* `identity.HashPassword(password string) (string, error)`
* `identity.CheckPassword(password, hash string) bool`
* `identity.GenerateJWT(userID, email, secret string, duration time.Duration) (string, error)`
* `identity.ValidateJWT(tokenStr, secret string) (*Claims, error)`

---

## 3. How to Test

```bash
go test -v ./pkg/identity
```
