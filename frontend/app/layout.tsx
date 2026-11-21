import "./globals.css";
import type { Metadata } from "next";
export const metadata: Metadata = {
  title: "VYRO | Dispatch console",
  description: "A mobility dispatch engineering prototype.",
};
export default function Layout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
