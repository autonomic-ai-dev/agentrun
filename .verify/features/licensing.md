# Feature Contract: Licensing

The repository is licensed under the Apache License, Version 2.0. The license text lives at the repository root, and the README points to it.

---

## Sub-features

### `license-text`
- **Intent**: State the license terms for everyone who receives the code.
- **Behavior**: `LICENSE` holds the unmodified Apache License 2.0 text. `README.md` links to it under "License".
- **Outcome**: Anyone reading the repository can find the terms without consulting another source.

---

## How to get to it (user POV)

- **Read**: `LICENSE`, or the "License" section of `README.md`.

---

## Code

- `LICENSE`
- `README.md` (license section only)

---

## Verification Suites (Targeted Test Commands)

- **Manual check**: `shasum -a 256 LICENSE` prints `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`, the checksum of the Apache-2.0 text as published at apache.org.
