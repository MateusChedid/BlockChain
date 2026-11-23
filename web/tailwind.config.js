/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    // This glob pattern is crucial for detecting Tailwind classes in your components
    "./src/**/*.{js,ts,jsx,tsx}", 
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}