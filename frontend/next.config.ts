import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  // Disable the X-Powered-By header
  poweredByHeader: false,
  // Allow images from any source (for file previews)
  images: {
    remotePatterns: [],
  },
};

export default nextConfig;
