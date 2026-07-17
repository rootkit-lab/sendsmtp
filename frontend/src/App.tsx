import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { AppLayout } from "@/components/AppLayout";
import { DashboardPage } from "@/pages/DashboardPage";
import { SmtpsPage } from "@/pages/SmtpsPage";
import { EmailsPage } from "@/pages/EmailsPage";
import { ContentPage } from "@/pages/ContentPage";
import { SettingsPage } from "@/pages/SettingsPage";

export default function App() {
  return (
    <HashRouter>
      <Toaster position="top-right" richColors />
      <Routes>
        <Route element={<AppLayout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/smtps" element={<SmtpsPage />} />
          <Route path="/emails" element={<EmailsPage />} />
          <Route path="/content" element={<ContentPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  );
}
