import { NavLink, Outlet } from "react-router-dom";
import { LayoutDashboard, Server, Mail, FileText, Settings } from "lucide-react";
import { cn } from "@/lib/utils";
import { useTranslation } from "@/i18n";

export function AppLayout() {
  const { t } = useTranslation();
  const nav = [
    { to: "/", label: t("nav.dashboard"), icon: LayoutDashboard },
    { to: "/smtps", label: t("nav.smtps"), icon: Server },
    { to: "/emails", label: t("nav.emails"), icon: Mail },
    { to: "/content", label: t("nav.content"), icon: FileText },
    { to: "/settings", label: t("nav.settings"), icon: Settings },
  ];

  return (
    <div className="flex h-full min-h-0">
      <aside className="flex w-56 shrink-0 flex-col border-r border-stone-300/80 bg-[#fffdf9]/80 backdrop-blur">
        <div className="border-b border-stone-300/80 px-5 py-6">
          <div className="font-[family-name:var(--font-display)] text-2xl tracking-tight text-stone-900">
            SendSMTP
          </div>
          <p className="mt-1 text-xs text-stone-500">{t("app.tagline")}</p>
        </div>
        <nav className="flex flex-1 flex-col gap-1 p-3">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2 rounded-md px-3 py-2 text-sm text-stone-700 transition-colors hover:bg-stone-200/70",
                  isActive && "bg-teal-900 text-white hover:bg-teal-900"
                )
              }
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="min-w-0 flex-1 overflow-auto p-6 md:p-8">
        <Outlet />
      </main>
    </div>
  );
}
