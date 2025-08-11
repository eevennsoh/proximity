/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ["class"],
  content: ["./index.html", "./src/**/*.{js,jsx,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        brand: {
          50: "#f0f7ff",
          100: "#dfeeff",
          200: "#b9dbff",
          300: "#8bc2ff",
          400: "#5aa5ff",
          500: "#2d88ff",
          600: "#126de6",
          700: "#0a57b8",
          800: "#0a4793",
          900: "#0c3c78"
        }
      },
      boxShadow: {
        card: "0 10px 25px -10px rgba(0,0,0,0.25)"
      }
    }
  },
  plugins: [require("tailwindcss-animate")]
};
