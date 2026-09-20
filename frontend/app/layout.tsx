import type { Metadata } from "next";
import Script from "next/script";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  metadataBase: new URL("https://golab.golangforall.in"),

  title: {
    default: "GoLab — Go Playground by GolangForAll",
    template: "%s | GoLab",
  },

  description:
    "GoLab is a browser-based Go playground by GolangForAll. Write Go, provide stdin, run your program, and get real compile and runtime feedback.",

  applicationName: "GoLab",

  keywords: [
    "Go",
    "Golang",
    "Go Playground",
    "Go Programming",
    "GolangForAll",
    "DSA",
    "Go Practice",
    "Go Compiler",
  ],

  authors: [
    {
      name: "GolangForAll",
      url: "https://golangforall.in",
    },
  ],

  creator: "GolangForAll",
  publisher: "GolangForAll",

  icons: {
    icon: "/logo.svg",
    shortcut: "/logo.svg",
    apple: "/logo.svg",
  },

  openGraph: {
    title: "GoLab — Go Playground by GolangForAll",
    description:
      "Write Go, provide stdin, and run your programs directly in the browser.",
    url: "https://golab.golangforall.in",
    siteName: "GoLab",
    type: "website",
    images: [
      {
        url: "/logo.svg",
        width: 512,
        height: 512,
        alt: "GoLab by GolangForAll",
      },
    ],
  },

  twitter: {
    card: "summary",
    title: "GoLab — Go Playground by GolangForAll",
    description:
      "A browser-based environment to write, run, and practice Go.",
  },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        {/* Google Analytics */}
        <Script
          async
          src="https://www.googletagmanager.com/gtag/js?id=G-084XZD6TYR"
        />

        <Script id="google-analytics">
          {`
            window.dataLayer = window.dataLayer || [];
            function gtag(){window.dataLayer.push(arguments);}
            gtag('js', new Date());
            gtag('config', 'G-084XZD6TYR');
          `}
        </Script>

        {children}
      </body>
    </html>
  );
}