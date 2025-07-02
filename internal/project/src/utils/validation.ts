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
  const validRoutingTypes: RoutingType[] = ['sni', 'http-header', 'tcp-raw', 'udp-raw'];
  if (!validRoutingTypes.includes(entryPoint.routing)) {
    errors.push({ field: 'routing', message: 'Invalid routing type' });
  }

  // Validate destination
  if (!entryPoint.to.trim()) {
    errors.push({ field: 'to', message: 'Destination is required' });
  } else if (!isValidDestination(entryPoint.to)) {
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

  // Validate block/allow lists for SNI routing
  if (entryPoint.routing === 'sni') {
    if (entryPoint.block_list) {
      entryPoint.block_list.forEach((domain, index) => {
        if (!isValidDomain(domain)) {
          errors.push({ 
            field: `block_list.${index}`, 
            message: 'Invalid domain format' 
          });
        }
      });
    }

    if (entryPoint.allow_list) {
      entryPoint.allow_list.forEach((domain, index) => {
        if (!isValidDomain(domain)) {
          errors.push({ 
            field: `allow_list.${index}`, 
            message: 'Invalid domain format' 
          });
        }
      });
    }
  }

  return errors;
};

const isValidListenAddress = (address: string): boolean => {
  const regex = /^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|localhost|0\.0\.0\.0):\d{1,5}$/;
  return regex.test(address);
};

const isValidDestination = (destination: string): boolean => {
  // Port only (e.g., "443") or host:port (e.g., "example.com:443")
  if (/^\d{1,5}$/.test(destination)) return true;
  const regex = /^[a-zA-Z0-9.-]+:\d{1,5}$/;
  return regex.test(destination);
};

const isValidTimeout = (timeout: string): boolean => {
  const regex = /^\d+[smh]$/;
  return regex.test(timeout);
};

const isValidProxyUrl = (url: string): boolean => {
  try {
    const parsed = new URL(url);
    return ['socks5:', 'ssh:'].includes(parsed.protocol);
  } catch {
    return false;
  }
};

const isValidDomain = (domain: string): boolean => {
  // Support wildcards and regular domains
  const regex = /^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
  return regex.test(domain);
};