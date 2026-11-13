"use client"

import { Download, Upload } from "lucide-react"
import { useEffect, useState } from "react"

import { useCurrentUser } from "@/components/app/app-provider"
import { DashboardDialog } from "@/components/dashboard/dashboard-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { adminGet } from "@/lib/api/admin"
import { apiFetch } from "@/lib/api/client"
import { PERMISSIONS } from "@/lib/auth/permissions.generated"
import { userHasPermission, userIsOwner } from "@/lib/auth/roles"
import { useI18n } from "@/lib/i18n/provider"

const MAX_FILE_SIZE = 5 * 1024 * 1024

type RoleOption = {
  id: number
  name?: string
  code?: string
  type?: number
}

type ImportSuccess = {
  row: number
  username: string
  email: string
  nickname: string
}

type ImportError = {
  row: number
  username: string
  message: string
}

type ImportResult = {
  created: number
  roleFailed: number
  failed: number
  success: ImportSuccess[]
  partial: ImportError[]
  errors: ImportError[]
}

export function ImportUsersDialog({
  onImported,
}: {
  onImported?: () => void
}) {
  const { t } = useI18n()
  const currentUser = useCurrentUser()
  const [open, setOpen] = useState(false)
  const [roles, setRoles] = useState<RoleOption[]>([])
  const [file, setFile] = useState<File | null>(null)
  const [defaultPassword, setDefaultPassword] = useState("")
  const [roleId, setRoleId] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<ImportResult | null>(null)
  const visibleRoles = userIsOwner(currentUser)
    ? roles
    : roles.filter((role) => role.type !== 0 && role.code !== "owner")

  useEffect(() => {
    if (!open) return
    void adminGet<RoleOption[]>("/api/admin/role/roles")
      .then((data) => setRoles(Array.isArray(data) ? data : []))
      .catch(() => setRoles([]))
  }, [open])

  if (!userHasPermission(currentUser, PERMISSIONS.DASHBOARD_USER_CREATE)) {
    return null
  }

  function openDialog() {
    setOpen(true)
    setError("")
    setResult(null)
    setFile(null)
    setDefaultPassword("")
    setRoleId("")
  }

  function selectFile(event: React.ChangeEvent<HTMLInputElement>) {
    const nextFile = event.target.files?.[0] ?? null
    setError("")
    setResult(null)
    if (!nextFile) {
      setFile(null)
      return
    }
    if (!nextFile.name.toLowerCase().endsWith(".xlsx")) {
      setError(t("dashboard.pages.users.importFileInvalid"))
      setFile(null)
      return
    }
    if (nextFile.size > MAX_FILE_SIZE) {
      setError(t("dashboard.pages.users.importFileTooLarge"))
      setFile(null)
      return
    }
    setFile(nextFile)
  }

  async function submit() {
    if (!file) {
      setError(t("dashboard.pages.users.importFileRequired"))
      return
    }
    setError("")
    setResult(null)
    setSubmitting(true)
    try {
      const formData = new FormData()
      formData.append("file", file)
      formData.append("defaultPassword", defaultPassword)
      formData.append("roleId", roleId)
      const data = await apiFetch<ImportResult>("/api/admin/user/import", {
        method: "POST",
        body: formData,
      })
      setResult(data)
      if (data.created + data.roleFailed > 0) {
        onImported?.()
      }
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t("dashboard.pages.users.importFailed")
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Button variant="outline" onClick={openDialog}>
        <Upload />
        {t("dashboard.pages.users.importUsers")}
      </Button>

      <DashboardDialog
        open={open}
        onOpenChange={setOpen}
        title={t("dashboard.pages.users.importTitle")}
        description={t("dashboard.pages.users.importHint")}
        size="xl"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={submitting}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              onClick={() => void submit()}
              disabled={submitting}
            >
              {submitting
                ? t("dashboard.pages.users.importUploading")
                : t("dashboard.pages.users.importSubmit")}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-3 rounded-md border bg-muted/30 p-3 text-sm">
            <Download className="size-4" />
            <span>{t("dashboard.pages.users.importTemplateHint")}</span>
            <a
              href="/api/admin/user/import_template"
              className="font-medium text-primary hover:underline"
              download
            >
              {t("dashboard.pages.users.importDownloadTemplate")}
            </a>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="import-file">
                {t("dashboard.pages.users.importFile")}
                <span className="text-destructive">*</span>
              </Label>
              <Input
                id="import-file"
                type="file"
                accept=".xlsx"
                disabled={submitting}
                onChange={selectFile}
              />
              <p className="text-xs text-muted-foreground">
                {t("dashboard.pages.users.importFileHint")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="import-default-password">
                {t("dashboard.pages.users.importDefaultPassword")}
                <span className="text-xs font-normal text-muted-foreground">
                  ({t("common.optional")})
                </span>
              </Label>
              <Input
                id="import-default-password"
                type="password"
                value={defaultPassword}
                disabled={submitting}
                onChange={(event) => setDefaultPassword(event.target.value)}
                placeholder={t(
                  "dashboard.pages.users.importDefaultPasswordPlaceholder"
                )}
              />
              <p className="text-xs text-muted-foreground">
                {t("dashboard.pages.users.importDefaultPasswordHint")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="import-role">
                {t("dashboard.pages.users.importRole")}
                <span className="text-xs font-normal text-muted-foreground">
                  ({t("common.optional")})
                </span>
              </Label>
              <select
                id="import-role"
                className="h-9 w-full rounded-md border bg-background px-3 text-sm"
                value={roleId}
                disabled={submitting}
                onChange={(event) => setRoleId(event.target.value)}
              >
                <option value="">
                  {t("dashboard.pages.users.importRolePlaceholder")}
                </option>
                {visibleRoles.map((role) => (
                  <option key={role.id} value={role.id}>
                    {role.name || role.code || role.id}
                  </option>
                ))}
              </select>
              <p className="text-xs text-muted-foreground">
                {t("dashboard.pages.users.importRoleHint")}
              </p>
            </div>
          </div>

          {error ? (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}

          {result ? (
            <div className="space-y-3" aria-live="polite">
              <div className="rounded-md border bg-muted/30 p-3 text-sm">
                <p className="font-medium">
                  {t("dashboard.pages.users.importSummary", {
                    created: result.created,
                    roleFailed: result.roleFailed,
                    failed: result.failed,
                  })}
                </p>
              </div>

              {result.success.length > 0 ? (
                <ImportTable
                  title={t("dashboard.pages.users.importSuccessList")}
                  rows={result.success}
                  columns={[
                    { key: "row", label: t("dashboard.pages.users.importRow") },
                    {
                      key: "username",
                      label: t("dashboard.pages.users.importUsername"),
                    },
                    { key: "email", label: t("dashboard.pages.users.importEmail") },
                    {
                      key: "nickname",
                      label: t("dashboard.pages.users.importNickname"),
                    },
                  ]}
                />
              ) : null}

              {result.partial.length > 0 ? (
                <ImportErrorTable
                  title={t("dashboard.pages.users.importPartialList")}
                  rows={result.partial}
                  messageLabel={t("dashboard.pages.users.importMessage")}
                  rowLabel={t("dashboard.pages.users.importRow")}
                  usernameLabel={t("dashboard.pages.users.importUsername")}
                />
              ) : null}

              {result.errors.length > 0 ? (
                <ImportErrorTable
                  title={t("dashboard.pages.users.importErrorList")}
                  rows={result.errors}
                  messageLabel={t("dashboard.pages.users.importMessage")}
                  rowLabel={t("dashboard.pages.users.importRow")}
                  usernameLabel={t("dashboard.pages.users.importUsername")}
                />
              ) : null}
            </div>
          ) : null}
        </div>
      </DashboardDialog>
    </>
  )
}

function ImportTable({
  title,
  rows,
  columns,
}: {
  title: string
  rows: Array<Record<string, string | number>>
  columns: Array<{ key: string; label: string }>
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{title}</p>
      <div className="max-h-52 overflow-auto rounded-md border">
        <table className="w-full text-left text-sm">
          <thead className="sticky top-0 bg-muted">
            <tr>
              {columns.map((column) => (
                <th key={column.key} className="px-3 py-2 font-medium">
                  {column.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={String(row.row)} className="border-t">
                {columns.map((column) => (
                  <td key={column.key} className="px-3 py-2">
                    {row[column.key]}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function ImportErrorTable({
  title,
  rows,
  rowLabel,
  usernameLabel,
  messageLabel,
}: {
  title: string
  rows: ImportError[]
  rowLabel: string
  usernameLabel: string
  messageLabel: string
}) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium text-destructive">{title}</p>
      <div className="max-h-52 overflow-auto rounded-md border">
        <table className="w-full text-left text-sm">
          <thead className="sticky top-0 bg-muted">
            <tr>
              <th className="px-3 py-2 font-medium">{rowLabel}</th>
              <th className="px-3 py-2 font-medium">{usernameLabel}</th>
              <th className="px-3 py-2 font-medium">{messageLabel}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={`${row.row}-${row.username}`} className="border-t">
                <td className="px-3 py-2">{row.row}</td>
                <td className="px-3 py-2">{row.username || "-"}</td>
                <td className="px-3 py-2">{row.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
