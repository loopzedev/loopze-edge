export interface NodeHelpProperty {
  key: string
  desc: string
}

export interface NodeHelpExample {
  title: string
  config: string
  result: string
}

export interface NodeHelpDoc {
  overview: string
  inputs?: string[]
  outputs?: string[]
  properties?: NodeHelpProperty[]
  examples?: NodeHelpExample[]
  tips?: string[]
}
