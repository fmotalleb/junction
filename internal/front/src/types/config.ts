export type RoutingType = 'sni' | 'http-header' | 'tcp-raw' | 'udp-raw';
export type ProxyProtocol = 'socks5' | 'ssh';

export interface EntryPoint {
  id: string;
  routing: RoutingType;
  listen: string;
  block_list?: string[];
  allow_list?: string[];
  proxy?: string[];
  to: string;
  timeout?: string;
}

export interface NetworkConfig {
  entrypoints: EntryPoint[];
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