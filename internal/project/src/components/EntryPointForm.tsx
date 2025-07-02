import React, { useState, useEffect } from 'react';
import { Save, X, AlertCircle } from 'lucide-react';
import { EntryPoint, RoutingType, ValidationError } from '../types/config';
import { validateEntryPoint } from '../utils/validation';
import { ArrayInput } from './ArrayInput';

interface EntryPointFormProps {
  entryPoint: EntryPoint;
  onSave: (entryPoint: EntryPoint) => void;
  onCancel: () => void;
  isEditing?: boolean;
}

export const EntryPointForm: React.FC<EntryPointFormProps> = ({
  entryPoint,
  onSave,
  onCancel,
  isEditing = false
}) => {
  const [formData, setFormData] = useState<EntryPoint>(entryPoint);
  const [errors, setErrors] = useState<ValidationError[]>([]);
  const [touched, setTouched] = useState<Record<string, boolean>>({});

  useEffect(() => {
    const validationErrors = validateEntryPoint(formData);
    setErrors(validationErrors);
  }, [formData]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    const validationErrors = validateEntryPoint(formData);
    if (validationErrors.length > 0) {
      setErrors(validationErrors);
      return;
    }

    onSave(formData);
  };

  const handleFieldChange = (field: keyof EntryPoint, value: any) => {
    setFormData({ ...formData, [field]: value });
    setTouched({ ...touched, [field]: true });
  };

  const getFieldError = (field: string): string | null => {
    const error = errors.find(e => e.field === field);
    return error && touched[field] ? error.message : null;
  };

  const routingTypes: { value: RoutingType; label: string; description: string }[] = [
    { value: 'sni', label: 'SNI', description: 'Server Name Indication routing' },
    { value: 'http-header', label: 'HTTP Header', description: 'Route based on HTTP headers' },
    { value: 'tcp-raw', label: 'TCP Raw', description: 'Raw TCP traffic routing' },
    { value: 'udp-raw', label: 'UDP Raw', description: 'Raw UDP traffic routing' }
  ];

  const showSniFields = formData.routing === 'sni';

  return (
    <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 z-50">
      <div className="bg-gray-800/90 backdrop-blur-xl rounded-xl shadow-2xl border border-gray-700/50 max-w-2xl w-full max-h-[90vh] overflow-auto">
        <div className="p-6 border-b border-gray-700/50">
          <h2 className="text-xl font-semibold text-white">
            {isEditing ? 'Edit Entry Point' : 'Add Entry Point'}
          </h2>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-6">
          {/* Routing Type */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Routing Type
            </label>
            <div className="grid grid-cols-2 gap-3">
              {routingTypes.map((type) => (
                <label
                  key={type.value}
                  className={`relative flex items-center p-4 border rounded-lg cursor-pointer transition-all duration-300 backdrop-blur-sm ${
                    formData.routing === type.value
                      ? 'border-pink-500/50 bg-gradient-to-r from-pink-500/10 to-purple-600/10'
                      : 'border-gray-600 hover:border-gray-500 hover:bg-gray-700/30'
                  }`}
                >
                  <input
                    type="radio"
                    name="routing"
                    value={type.value}
                    checked={formData.routing === type.value}
                    onChange={(e) => handleFieldChange('routing', e.target.value as RoutingType)}
                    className="sr-only"
                  />
                  <div>
                    <div className="font-medium text-sm text-white">{type.label}</div>
                    <div className="text-xs text-gray-400">{type.description}</div>
                  </div>
                </label>
              ))}
            </div>
          </div>

          {/* Listen Address */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Listen Address
            </label>
            <input
              type="text"
              value={formData.listen}
              onChange={(e) => handleFieldChange('listen', e.target.value)}
              onBlur={() => setTouched({ ...touched, listen: true })}
              placeholder="0.0.0.0:8443"
              className={`w-full px-3 py-2 bg-gray-700/50 border rounded-lg focus:ring-2 focus:ring-pink-500 focus:border-transparent text-white placeholder-gray-400 backdrop-blur-sm transition-all duration-300 ${
                getFieldError('listen') ? 'border-red-500/50' : 'border-gray-600'
              }`}
            />
            {getFieldError('listen') && (
              <p className="mt-1 text-sm text-red-400 flex items-center gap-1">
                <AlertCircle className="w-4 h-4" />
                {getFieldError('listen')}
              </p>
            )}
          </div>

          {/* Destination */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Destination
            </label>
            <input
              type="text"
              value={formData.to}
              onChange={(e) => handleFieldChange('to', e.target.value)}
              onBlur={() => setTouched({ ...touched, to: true })}
              placeholder="443 or example.com:443"
              className={`w-full px-3 py-2 bg-gray-700/50 border rounded-lg focus:ring-2 focus:ring-pink-500 focus:border-transparent text-white placeholder-gray-400 backdrop-blur-sm transition-all duration-300 ${
                getFieldError('to') ? 'border-red-500/50' : 'border-gray-600'
              }`}
            />
            {getFieldError('to') && (
              <p className="mt-1 text-sm text-red-400 flex items-center gap-1">
                <AlertCircle className="w-4 h-4" />
                {getFieldError('to')}
              </p>
            )}
          </div>

          {/* Timeout */}
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Timeout (optional)
            </label>
            <input
              type="text"
              value={formData.timeout || ''}
              onChange={(e) => handleFieldChange('timeout', e.target.value)}
              placeholder="30s, 5m, 1h"
              className="w-full px-3 py-2 bg-gray-700/50 border border-gray-600 rounded-lg focus:ring-2 focus:ring-pink-500 focus:border-transparent text-white placeholder-gray-400 backdrop-blur-sm transition-all duration-300"
            />
          </div>

          {/* Proxy URLs */}
          <ArrayInput
            label="Proxy URLs"
            values={formData.proxy || []}
            onChange={(values) => handleFieldChange('proxy', values)}
            placeholder="socks5://10.11.12.22:8999"
            validation={(value) => {
              try {
                const parsed = new URL(value);
                if (!['socks5:', 'ssh:'].includes(parsed.protocol)) {
                  return 'Only socks5:// and ssh:// protocols are supported';
                }
                return null;
              } catch {
                return 'Invalid URL format';
              }
            }}
          />

          {/* SNI-specific fields */}
          {showSniFields && (
            <>
              <ArrayInput
                label="Block List"
                values={formData.block_list || []}
                onChange={(values) => handleFieldChange('block_list', values)}
                placeholder="api.google.com"
                validation={(value) => {
                  const domainRegex = /^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
                  return domainRegex.test(value) ? null : 'Invalid domain format';
                }}
              />

              <ArrayInput
                label="Allow List"
                values={formData.allow_list || []}
                onChange={(values) => handleFieldChange('allow_list', values)}
                placeholder="*.google.com"
                validation={(value) => {
                  const domainRegex = /^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
                  return domainRegex.test(value) ? null : 'Invalid domain format';
                }}
              />
            </>
          )}

          {/* Actions */}
          <div className="flex justify-end gap-3 pt-4 border-t border-gray-700/50">
            <button
              type="button"
              onClick={onCancel}
              className="px-4 py-2 text-gray-300 bg-gray-700/50 hover:bg-gray-600/50 rounded-lg transition-all duration-300 flex items-center gap-2 backdrop-blur-sm"
            >
              <X className="w-4 h-4" />
              Cancel
            </button>
            <button
              type="submit"
              disabled={errors.length > 0}
              className="px-4 py-2 bg-gradient-to-r from-pink-500 to-purple-600 text-white hover:from-pink-600 hover:to-purple-700 disabled:from-gray-600 disabled:to-gray-700 disabled:cursor-not-allowed rounded-lg transition-all duration-300 flex items-center gap-2 shadow-lg hover:shadow-pink-500/25"
            >
              <Save className="w-4 h-4" />
              {isEditing ? 'Update' : 'Add'} Entry Point
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};