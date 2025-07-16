// hooks/usePortal.tsx
import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';

export const usePortal = (id = 'modal-root') => {
  const [container, setContainer] = useState<HTMLElement | null>(null);

  useEffect(() => {  
    let created = false;  
    let element = document.getElementById(id);  
    if (!element) {  
      element = document.createElement('div');  
      element.id = id;  
      document.body.appendChild(element);  
      created = true;  
    }  
    setContainer(element);  

    return () => {  
      if (created && element && element.parentNode) {  
        element.parentNode.removeChild(element);  
      }  
    };  
  }, [id]);
  return (children: React.ReactNode) =>
    container ? createPortal(children, container) : null;
};
