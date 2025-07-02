import React, { useState } from 'react';
import { GripVertical } from 'lucide-react';
import { EntryPoint } from '../types/config';

interface DragDropListProps {
  items: EntryPoint[];
  onReorder: (items: EntryPoint[]) => void;
  renderItem: (item: EntryPoint, index: number) => React.ReactNode;
}

export const DragDropList: React.FC<DragDropListProps> = ({ 
  items, 
  onReorder, 
  renderItem 
}) => {
  const [draggedIndex, setDraggedIndex] = useState<number | null>(null);
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null);

  const handleDragStart = (e: React.DragEvent, index: number) => {
    setDraggedIndex(index);
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragOver = (e: React.DragEvent, index: number) => {
    e.preventDefault();
    setDragOverIndex(index);
  };

  const handleDragEnd = () => {
    setDraggedIndex(null);
    setDragOverIndex(null);
  };

  const handleDrop = (e: React.DragEvent, dropIndex: number) => {
    e.preventDefault();
    
    if (draggedIndex === null || draggedIndex === dropIndex) return;

    const newItems = [...items];
    const draggedItem = newItems[draggedIndex];
    
    newItems.splice(draggedIndex, 1);
    newItems.splice(dropIndex, 0, draggedItem);
    
    onReorder(newItems);
    setDraggedIndex(null);
    setDragOverIndex(null);
  };

  return (
    <div className="space-y-4">
      {items.map((item, index) => (
        <div
          key={item.id}
          className={`relative transition-all duration-300 ${
            draggedIndex === index ? 'opacity-50 scale-95' : ''
          } ${
            dragOverIndex === index && draggedIndex !== index 
              ? 'transform translate-y-1' 
              : ''
          }`}
          draggable
          onDragStart={(e) => handleDragStart(e, index)}
          onDragOver={(e) => handleDragOver(e, index)}
          onDragEnd={handleDragEnd}
          onDrop={(e) => handleDrop(e, index)}
        >
          <div className="absolute left-2 top-1/2 transform -translate-y-1/2 z-10">
            <GripVertical className="w-4 h-4 text-gray-500 cursor-grab active:cursor-grabbing hover:text-pink-400 transition-colors duration-300" />
          </div>
          <div className="pl-8">
            {renderItem(item, index)}
          </div>
        </div>
      ))}
    </div>
  );
};