import { Outlet } from '@tanstack/react-router';
import { MobileSidebarNav, Sidebar } from './Sidebar';

export function AppLayout() {
  return (
    <div className="flex h-screen overflow-hidden bg-[#09101c]">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-white/6 px-4 md:hidden">
          <div className="font-mono text-[12px] tracking-[0.22em] text-slate-100">
            /deployik
          </div>
          <MobileSidebarNav />
        </header>
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
