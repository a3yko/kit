# storage

A thin wrapper over an S3-compatible object store — **Cloudflare R2**, AWS S3,
MinIO — for the handful of operations an app actually needs: put, get, delete,
and presigned URLs.

Built on `aws-sdk-go-v2`, with the checksum knobs R2 requires already set
(`aws-sdk-go-v2` sends flexible checksums by default that R2 rejects).

## Install

```sh
go get github.com/a3yko/kit/storage
```

## Construct

```go
// Cloudflare R2 (endpoint built from the account id, region "auto"):
b := storage.NewR2(accountID, accessKeyID, secretAccessKey, "documents")

// Any S3-compatible endpoint:
b := storage.New(storage.Config{
    Endpoint:        "https://s3.eu-central-1.amazonaws.com",
    Region:          "eu-central-1",
    AccessKeyID:     id,
    SecretAccessKey: secret,
    Bucket:          "documents",
})
```

## Usage

```go
// Upload (server-side):
err := b.Put(ctx, "vehicles/abc/title.pdf", file, "application/pdf")

// Download:
rc, err := b.Get(ctx, "vehicles/abc/title.pdf")
defer rc.Close()

// Delete (deleting a missing key is not an error):
err := b.Delete(ctx, "vehicles/abc/title.pdf")
```

## Presigned URLs

Serve private files without proxying bytes through your app, or let the browser
upload straight to the bucket:

```go
// Short-lived download link (e.g. for an <a href> or <img src>):
url, _ := b.PresignGet(ctx, "vehicles/abc/title.pdf", 15*time.Minute)

// Direct browser → bucket upload (the bytes never touch your server):
url, _ := b.PresignPut(ctx, "vehicles/abc/new.pdf", "application/pdf", 10*time.Minute)
```

## Notes

- Keep buckets **private** and serve via `PresignGet` — the URL expires, so the
  object stays protected.
- Direct browser uploads via `PresignPut` need **CORS** configured on the bucket;
  server-side `Put` does not.
- Presigning is done locally (no network), so it's cheap and testable offline.
