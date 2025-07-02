import React from 'react';
import { Edit2, Trash2, Server, Globe, Shield, Clock } from 'lucide-react';
import { EntryPoint } from '../types/config';

interface EntryPointCardProps {
  entryPoint: EntryPoint;
  onEdit: () => void;
  onDelete: () => void;
}

export const EntryPointCard: React.FC<EntryPointCardProps> = ({
  entryPoint,
  onEdit,
  onDelete
}) => {
  const getRoutingIcon = (routing: string) => {
    switch (routing) {
      case 'sni': return <Shield className="w-4 h-4" />;
      case 'http-header': return <Globe className="w-4 h-4" />;
      default: return <Server className="w-4 h-4" />;
    }
  };

  const getRoutingColor = (routing: string) => {
    switch (routing) {
      case 'sni': return 'bg-gradient-to-r from-emerald-500/20 to-green-600/20 text-emerald-300 border-emerald-500/30';
      case 'http-header': return 'bg-gradient-to-r from-blue-500/20 to-cyan-600/20 text-blue-300 border-blue-500/30';
      case 'tcp-raw': return 'bg-gradient-to-r from-purple-500/20 to-violet-600/20 text-purple-300 border-purple-500/30';
      case 'udp-raw': return 'bg-gradient-to-r from-orange-500/20 to-amber-600/20 text-orange-300 border-orange-500/30';
      default: return 'bg-gradient-to-r from-gray-500/20 to-slate-600/20 text-gray-300 border-gray-500/30';
    }
  };

  return (
    <div className="bg-gray-800/50 backdrop-blur-sm rounded-lg border border-gray-700/50 p-6 hover:bg-gray-800/70 hover:border-gray-600/50 transition-all duration-300 hover:shadow-lg hover:shadow-pink-500/10">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className={`inline-flex items-center gap-1 px-3 py-1 rounded-full text-xs font-medium border backdrop-blur-sm ${getRoutingColor(entryPoint.routing)}`}>
            {getRoutingIcon(entryPoint.routing)}
            {entryPoint.routing.toUpperCase()}
          </div>
        </div>
        
        <div className="flex items-center gap-2">
          <button
            onClick={onEdit}
            className="p-2 text-gray-400 hover:text-pink-400 hover:bg-pink-500/10 rounded-lg transition-all duration-300"
            title="Edit entry point"
          >
            <Edit2 className="w-4 h-4" />
          </button>
          <button
            onClick={onDelete}
            className="p-2 text-gray-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-all duration-300"
            title="Delete entry point"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        </div>
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-400">Listen:</span>
          <span className="text-sm font-mono bg-gray-700/50 text-gray-200 px-2 py-1 rounded backdrop-blur-sm">
            {entryPoint.listen}
          </span>
        </div>

        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-400">Destination:</span>
          <span className="text-sm font-mono bg-gray-700/50 text-gray-200 px-2 py-1 rounded backdrop-blur-sm">
            {entryPoint.to}
          </span>
        </div>

        {entryPoint.timeout && (
          <div className="flex items-center justify-between">
            <span className="text-sm text-gray-400 flex items-center gap-1">
              <Clock className="w-3 h-3" />
              Timeout:
            </span>
            <span className="text-sm font-mono bg-gray-700/50 text-gray-200 px-2 py-1 rounded backdrop-blur-sm">
              {entryPoint.timeout}
            </span>
          </div>
        )}

        {entryPoint.proxy && entryPoint.proxy.length > 0 && (
          <div>
            <span className="text-sm text-gray-400 block mb-1">Proxies:</span>
            <div className="space-y-1">
              {entryPoint.proxy.map((proxy, index) => (
                <div key={index} className="text-xs font-mono bg-gradient-to-r from-blue-500/10 to-cyan-600/10 text-blue-300 px-2 py-1 rounded border border-blue-500/20 backdrop-blur-sm">
                  {proxy}
                </div>
              ))}
            </div>
          </div>
        )}

        {entryPoint.routing === 'sni' && (
          <>
            {entryPoint.block_list && entryPoint.block_list.length > 0 && (
              <div>
                <span className="text-sm text-gray-400 block mb-1">Blocked:</span>
                <div className="flex flex-wrap gap-1">
                  {entryPoint.block_list.map((domain, index) => (
                    <span key={index} className="text-xs bg-gradient-to-r from-red-500/10 to-pink-600/10 text-red-300 px-2 py-1 rounded border border-red-500/20 backdrop-blur-sm">
                      {domain}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {entryPoint.allow_list && entryPoint.allow_list.length > 0 && (
              <div>
                <span className="text-sm text-gray-400 block mb-1">Allowed:</span>
                <div className="flex flex-wrap gap-1">
                  {entryPoint.allow_list.map((domain, index) => (
                    <span key={index} className="text-xs bg-gradient-to-r from-green-500/10 to-emerald-600/10 text-green-300 px-2 py-1 rounded border border-green-500/20 backdrop-blur-sm">
                      {domain}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};