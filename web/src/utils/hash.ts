import { sha256 } from "hash-wasm"

// TODO: use argon2
const hash_salt = "https://github.com/alist-org/alist"

export function hashPwd(pwd: string): Promise<string> {
  return sha256(`${pwd}-${hash_salt}`)
}
