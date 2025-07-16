import React, { useState } from 'react';
import { Plus, X } from 'lucide-react';
import { DragDropList } from './DragDropList';

interface ArrayInputProps {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  validation?: (value: string) => string | null;
}

export const ArrayInput: React.FC<ArrayInputProps> = ({
  label,
  values,
  onChange,
  placeholder = '',
  validation,
}) => {
  const [newValue, setNewValue] = useState('');
  const [errors, setErrors] = useState<Record<number, string>>({});

  const addValue = () => {
    if (!newValue.trim()) return;

    const error = validation?.(newValue);
    if (error) {
      setErrors({ ...errors, [-1]: error });
      return;
    }

    onChange([...values, newValue.trim()]);
    setNewValue('');
    setErrors({});
  };

  const removeValue = (index: number) => {
    const newValues = values.filter((_, i) => i !== index);
    onChange(newValues);

    const newErrors = { ...errors };
    delete newErrors[index];
    setErrors(newErrors);
  };

  const handleReorder = (newValues: { id: string; value: string }[]) => {
    const reorderedValues = newValues.map((item) => item.value);
    onChange(reorderedValues);
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addValue();
    }
  };

  return (
    <div className="space-y-3">
      <label className="block text-sm font-medium text-gray-300">{label}</label>

      {/* Existing values with DragDropList */}
      <DragDropList<{ id: string; value: string }>
        items={values.map((value, index) => ({ id: index.toString(), value }))}
        onReorder={handleReorder}
        renderItem={(item, index) => (
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={item.value}
              onChange={(e) => {
                const newValues = [...values];
                newValues[index] = e.target.value;
                onChange(newValues);
              }}
              className="flex-1 px-3 py-2 bg-gray-700/50 border border-gray-600 rounded-lg focus:ring-2 focus:ring-pink-500 focus:border-transparent text-white placeholder-gray-400 text-sm  transition-all duration-300"
            />
            <button
              type="button"
              onClick={() => removeValue(index)}
              className="p-2 text-red-400 hover:bg-red-500/10 rounded-lg transition-all duration-300"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        )}
      />

      {/* Add new value */}
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={newValue}
          onChange={(e) => setNewValue(e.target.value)}
          onKeyPress={handleKeyPress}
          placeholder={placeholder}
          className={`flex-1 px-3 py-2 bg-gray-700/50 border rounded-lg focus:ring-2 focus:ring-pink-500 focus:border-transparent text-white placeholder-gray-400 text-sm  transition-all duration-300 ${errors[-1] ? 'border-red-500/50' : 'border-gray-600'
            }`}
        />
        <button
          type="button"
          onClick={addValue}
          className="p-2 bg-gradient-to-r from-pink-500 to-purple-600 text-white hover:from-pink-600 hover:to-purple-700 rounded-lg transition-all duration-300 shadow-lg hover:shadow-pink-500/25"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>

      {errors[-1] && <p className="text-sm text-red-400">{errors[-1]}</p>}
    </div>
  );
};