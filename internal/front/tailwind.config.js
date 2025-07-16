/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      keyframes: {
        fadeIn: {
          '0%': { opacity: 0 },
          '100%': { opacity: 1 },
        },
        scaleIn: {
          '0%': { transform: 'scale(0.95)', opacity: 0 },
          '100%': { transform: 'scale(1)', opacity: 1 },
        },
      },
      animation: {
        fadeIn: 'fadeIn 300ms ease-out forwards',
        scaleIn: 'scaleIn 300ms ease-out forwards',
      },
      colors: {
        scrollbar: {
          track: '#1f2937',
          thumb: '#4b5563',
        },
      },
    },
  },
  plugins: [
    require('tailwind-scrollbar'),
  ],
};
