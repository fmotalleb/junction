// hooks/usePortal.tsx
import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';

export const usePortal = (id = 'modal-root') => {
  const [container, setContainer] = useState<HTMLElement | null>(null);

  useEffect(() => {
    let element = document.getElementById(id);
    if (!element) {
      element = document.createElement('div');
      element.id = id;
      document.body.appendChild(element);
    }
    setContainer(element);
  }, [id]);

  return (children: React.ReactNode) =>
    container ? createPortal(children, container) : null;
};
