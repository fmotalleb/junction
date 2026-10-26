import { EntryPoint, ValidationError, RoutingType } from '../types/config';

export const validateEntryPoint = (entryPoint: EntryPoint): ValidationError[] => {
  const errors: ValidationError[] = [];

  // Validate listen address
  if (!entryPoint.listen.trim()) {
    errors.push({ field: 'listen', message: 'Listen address is required' });
  } else if (!isValidListenAddress(entryPoint.listen)) {
    errors.push({ field: 'listen', message: 'Invalid listen address format (e.g., 0.0.0.0:8443)' });
  }
  // Validate routing type
  const validRoutingTypes: RoutingType[] = ['sni', 'http-header', 'http-to-https', 'tcp-raw', 'udp-raw'];
  if (!validRoutingTypes.includes(entryPoint.routing)) {
    errors.push({ field: 'routing', message: 'Invalid routing type' });
  }

  // Validate destination
  if (!entryPoint.to.trim()) {
    errors.push({ field: 'to', message: 'Destination is required' });
  } else if (entryPoint.routing === 'http-to-https' ? !isValidHttpsURL(entryPoint.to) : !isValidDestination(entryPoint.to, entryPoint.routing)) {
    errors.push({ field: 'to', message: 'Invalid destination format (e.g., 443 or example.com:443)' });
  }

  // Validate timeout format
  if (entryPoint.timeout && !isValidTimeout(entryPoint.timeout)) {
    errors.push({ field: 'timeout', message: 'Invalid timeout format (e.g., 30s, 5m)' });
  }

  // Validate proxy URLs
  if (entryPoint.proxy) {
    entryPoint.proxy.forEach((proxyUrl, index) => {
      if (!isValidProxyUrl(proxyUrl)) {
        errors.push({ 
          field: `proxy.${index}`, 
          message: 'Invalid proxy URL format (e.g., socks5://host:port, ssh://user@host:port)' 
        });
      }
    });
  }

  for (const field of ['destination_allow', 'destination_block'] as const) {
    for (const cidr of entryPoint[field] || []) {
      if (!isValidCIDR(cidr)) {
        errors.push({ field, message: `Invalid CIDR: ${cidr}` });
      }
    }
  }
  if ((entryPoint.destination_allow?.length || entryPoint.destination_block?.length) && entryPoint.proxy?.length) {
    errors.push({ field: 'proxy', message: 'Destination CIDR rules require a direct route without proxies' });
  }
  for (const port of entryPoint.allowed_ports || []) {
    if (!isValidPort(port)) {
      errors.push({ field: 'allowed_ports', message: `Invalid allowed port: ${port}` });
    }
  }
  return errors;
};
const isValidListenAddress = (address: string): boolean => {
  const portOnly = address.match(/^:?([0-9]{1,5})$/);
  if (portOnly) return isValidPort(portOnly[1]);

  const match = address.match(/^(\[[0-9a-fA-F:.%]+\]|(?:[0-9]{1,3}\.){3}[0-9]{1,3}):([0-9]{1,5})$/);
  if (!match || !isValidPort(match[2])) return false;
  const host = match[1];
  if (!host.startsWith('[')) return host.split('.').every((octet) => Number(octet) <= 255);
  try {
    return new URL(`http://${address}`).hostname !== '';
  } catch {
    return false;
  }
};

const isValidDestination = (destination: string, routing: RoutingType): boolean => {
  if (routing === 'sni' || routing === 'http-header') return isValidPort(destination);
  if (routing !== 'tcp-raw' && routing !== 'udp-raw') return false;
  try {
    const parsed = new URL(`udp://${destination}`);
    return parsed.hostname !== '' && isValidPort(parsed.port) && parsed.username === '' && parsed.password === '' && parsed.pathname === '/' && parsed.search === '' && parsed.hash === '';
  } catch {
    return false;
  }
};

const isValidTimeout = (timeout: string): boolean => {
  if (timeout === '0') return true;
  const regex = /^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$/;
  return regex.test(timeout);
};

const isValidPort = (value: string): boolean => {
  if (!/^\d{1,5}$/.test(value)) return false;
  const port = Number(value);
  return port >= 1 && port <= 65535;
};

const isValidProxyUrl = (url: string): boolean => {
  try {
    const parsed = new URL(url);
    return parsed.hostname !== '';
  } catch {
    return false;
  }
};

const isValidHttpsURL = (value: string): boolean => {
  try {
    const parsed = new URL(value);
    return parsed.protocol === 'https:' && parsed.hostname !== '' && parsed.username === '' && parsed.password === '';
  } catch {
    return false;
  }
};

const isValidCIDR = (value: string): boolean => {
  const parts = value.trim().split('/');
  if (parts.length !== 2 || !/^\d+$/.test(parts[1])) return false;
  const prefix = Number(parts[1]);
  const address = parts[0];
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(address)) {
    return prefix <= 32 && address.split('.').every((octet) => Number(octet) <= 255);
  }
  if (!address.includes(':') || prefix > 128) return false;
  try {
    new URL(`http://[${address}]/`);
    return true;
  } catch {
    return false;
  }
};
