# Releasing to the Terraform Registry

This fork publishes as **`mohalsherif/metabase`** on the public
[Terraform Registry](https://registry.terraform.io/). Consumers then use:

```hcl
terraform {
  required_providers {
    metabase = {
      source  = "mohalsherif/metabase"
      version = "~> 0.14"
    }
  }
}
```

> Note the address change: upstream is `flovouin/metabase`. Anything currently
> pinned to the upstream address must have its `required_providers` block
> updated (and, if a state already records the old provider, a
> `terraform state replace-provider flovouin/metabase mohalsherif/metabase`).

## There is no "registry token"

Provider publishing is **not** a push to an artifact store, so there is nothing
to authenticate against and no API token to create. The flow is:

1. You authorise the Terraform Registry's GitHub OAuth app once, and publish the
   repository through the Registry UI.
2. The Registry installs a webhook on this repository.
3. Every time a **GitHub release** appears with a `v`-prefixed semver tag and the
   expected signed assets, the Registry ingests it automatically.

So the pipeline's whole job is to produce a correct GitHub release. That is what
[`.github/workflows/release.yml`](.github/workflows/release.yml) does, driven by
[`.goreleaser.yml`](.goreleaser.yml).

(An HCP Terraform / Terraform Enterprise **private** registry is a different
product and *does* use an API token. That is not what this repository targets.)

## One-time setup

### 1. Generate a GPG signing key

The Registry verifies the signature on every release. It accepts **RSA or DSA
keys only — not the default ECC type**, so the key type matters.

```bash
gpg --full-generate-key
# 1 → "RSA and RSA"
# 4096 → key size
# 0 → never expires
# Use a real name/email, and SET A PASSPHRASE (the workflow expects one)
```

Find the key id and export both halves:

```bash
gpg --list-secret-keys --keyid-format=long
gpg --armor --export "<KEY_ID>"              # public  → Registry signing key
gpg --armor --export-secret-keys "<KEY_ID>"  # private → GitHub secret
```

### 2. Add the GitHub Actions secrets

In **Settings → Secrets and variables → Actions** on this repository:

| Secret            | Value                                                                       |
| ----------------- | --------------------------------------------------------------------------- |
| `GPG_PRIVATE_KEY` | Full armored private key, including the `-----BEGIN`/`-----END` lines        |
| `PASSPHRASE`      | The passphrase for that key                                                  |

`GITHUB_TOKEN` is provided automatically — nothing to create.

The release workflow fails immediately with a clear message if `GPG_PRIVATE_KEY`
is missing, rather than failing deep inside GoReleaser.

### 3. Register the public key and publish the provider

1. Sign in to <https://registry.terraform.io/> **with GitHub** — there is no
   separate Registry account to create, and the GitHub account must have admin
   or push access to this repository.
2. **User Settings → Signing Keys → + New GPG Key** — paste the *public* key.
3. **Publish → Provider** — pick the `mohalsherif` namespace and this
   repository, then accept the terms.

The publish step requires at least one existing release, so do step 4 first if
the repository has none yet; the Registry will pick up all later releases via
the webhook.

## Cutting a release

```bash
git checkout main && git pull
git tag v0.14.3
git push origin v0.14.3
```

Pushing the tag is the only manual step. **Never `git push --tags`** — every
`v*` tag fires the release workflow.

The workflow builds every OS/arch, writes the checksum file, detaches a GPG
signature over it, attaches the registry manifest, creates the GitHub release,
and finally asserts that the exact asset set the Registry requires is present:

- `terraform-provider-metabase_<version>_<os>_<arch>.zip`
- `terraform-provider-metabase_<version>_SHA256SUMS`
- `terraform-provider-metabase_<version>_SHA256SUMS.sig`
- `terraform-provider-metabase_<version>_manifest.json`

A broken `.goreleaser.yml` is caught on pull requests by the
`📦 Validate release config` job (`goreleaser check` + a snapshot build), so a
config error never strands a pushed tag.

### If a release fails

Delete the release and the tag, fix the problem, and re-tag:

```bash
gh release delete v0.14.3 --yes
git push --delete origin v0.14.3
git tag -d v0.14.3
```

The Registry ignores versions whose assets are incomplete; once a version has
been ingested successfully it is immutable, so bump the patch version instead of
re-cutting it.
