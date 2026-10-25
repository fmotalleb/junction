export type RoutingType = 'sni' | 'http-header' | 'http-to-https' | 'tcp-raw' | 'udp-raw';
export type ProxyProtocol = 'socks5' | 'ssh';

export interface EntryPoint {
  id: string;
  routing: RoutingType;
  listen: string;
  block_list?: string[];
  allow_list?: string[];
  proxy?: string[];
  allow_from?: string[];
  block_from?: string[];
  destination_allow?: string[];
  destination_block?: string[];
  tag?: string;
  features?: string[];
  extra?: Record<string, unknown>;
  to: string;
  timeout?: string;
  [key: string]: unknown;
}

export interface NetworkConfig {
  entrypoints: EntryPoint[];
  core?: Record<string, unknown>;
  include?: string[];
  [key: string]: unknown;
}

export interface ValidationError {
  field: string;
  message: string;
}

export interface FormState {
  isValid: boolean;
  errors: ValidationError[];
  touched: Record<string, boolean>;
}
