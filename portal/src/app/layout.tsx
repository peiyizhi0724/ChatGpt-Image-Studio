import { Toaster } from "sonner";
import "./globals.css";
import { TopNav } from "@/components/top-nav";

export default function AppShell({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <>
      <Toaster position="top-center" richColors />
      <main
        className="box-border min-h-screen overflow-y-auto bg-[#f5f5f3] p-3 text-stone-900 lg:h-full lg:min-h-0 lg:overflow-hidden lg:p-4"
        style={{
          fontFamily:
            '"SF Pro Display","SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif',
        }}
      >
        <div className="mx-auto flex min-h-full max-w-[1680px] flex-col gap-3 lg:h-full lg:min-h-0 lg:flex-row lg:gap-4">
          <TopNav />
          <div className="min-w-0 flex-1 lg:min-h-0">{children}</div>
        </div>
      </main>
    </>
  );
}
