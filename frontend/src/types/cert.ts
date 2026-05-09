// Mirror of the wire-safe CertEntrySummary returned by the Go backend.
// PEM material and private keys are intentionally absent — the server
// never sends them. CertPath / KeyPath ARE returned for file-source
// entries because operators legitimately need to see where a stored
// reference points.

export type CertType = 'ca-bundle' | 'client-pair' | 'server-pair'
export type CertSource = 'inline' | 'file'

export interface CertEntrySummary {
  id: string
  name: string
  type: CertType
  source: CertSource
  certPath?: string
  keyPath?: string
  fingerprint?: string
  subject?: string
  issuer?: string
  notBefore?: string
  notAfter?: string
  notes?: string
  createdAt?: string
  updatedAt?: string
}

// CertEntryInput is the shape consumed by POST/PUT /api/v1/certs and the
// /validate probe endpoint. PEM material is included; the backend stores
// it encrypted at rest. For Source: file the PEM fields are omitted and
// CertPath / KeyPath carry the absolute paths instead.
export interface CertEntryInput {
  id?: string
  name?: string
  type: CertType
  source: CertSource
  certPem?: string
  keyPem?: string
  certPath?: string
  keyPath?: string
  notes?: string
}

// CertReference is one occurrence of a cert ID inside the workspace,
// returned in the 409 body when a DELETE is blocked.
export interface CertReference {
  flowId?: string
  nodeId: string
  nodeType: string
  field: string
}

export const CERT_TYPE_OPTIONS: { value: CertType; label: string }[] = [
  { value: 'ca-bundle', label: 'CA bundle' },
  { value: 'client-pair', label: 'Client cert + key' },
  { value: 'server-pair', label: 'Server cert + key' },
]

export const CERT_SOURCE_OPTIONS: { value: CertSource; label: string }[] = [
  { value: 'inline', label: 'Inline PEM (encrypted at rest)' },
  { value: 'file', label: 'File path (operator-managed)' },
]
