import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
  basePath: process.env.NODE_ENV === "production" ? "/agent-farmer" : "",
  /* config options here */
};

export default nextConfig;