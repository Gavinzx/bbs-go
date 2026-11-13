"use client"

import { UserPlus } from "lucide-react"
import { useState } from "react"

import { useCurrentUser } from "@/components/app/app-provider"
import { DashboardDialog } from "@/components/dashboard/dashboard-dialog"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { adminPostJson } from "@/lib/api/admin"
import { PERMISSIONS } from "@/lib/auth/permissions.generated"
import { userHasPermission } from "@/lib/auth/roles"
import { useI18n } from "@/lib/i18n/provider"

const MAX_BATCH_USERS = 100

type BatchUser = {
  username: string
  email: string
  nickname: string
  password: string
}

type BatchCreateError = {
  index: number
  username: string
  message: string
}

type BatchCreateResult = {
  created: number
  roleFailed?: number
  failed: number
  errors?: BatchCreateError[]
}

function parseUsers(text: string): { users: BatchUser[]; errorIndex?: number } {
  const lines = text.split(/\r?\n/)
  const users: BatchUser[] = []

  for (const rawLine of lines) {
    const line = rawLine.trim()
    if (!line) continue

    const parts = line.split(/[,，]/).map((part) => part.trim())
    if (parts.length !== 4) {
      return { users: [], errorIndex: users.length + 1 }
    }

    const [username, email, nickname, password] = parts
    if (!username || !email || !nickname || !password) {
      return { users: [], errorIndex: users.length + 1 }
    }
    users.push({ username, email, nickname, password })
  }

  return { users }
}

export function BatchCreateUserDialog({
  onCreated,
}: {
  onCreated?: () => void
}) {
  const { t } = useI18n()
  const currentUser = useCurrentUser()
  const [open, setOpen] = useState(false)
  const [text, setText] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<BatchCreateResult | null>(null)

  if (!userHasPermission(currentUser, PERMISSIONS.DASHBOARD_USER_CREATE)) {
    return null
  }

  function openDialog() {
    setOpen(true)
    setError("")
    setResult(null)
  }

  async function submit() {
    setError("")
    setResult(null)

    const parsed = parseUsers(text)
    if (parsed.errorIndex) {
      setError(
        t("dashboard.pages.users.batchInvalidLine", {
          index: parsed.errorIndex,
        })
      )
      return
    }
    if (parsed.users.length === 0) {
      setError(t("dashboard.pages.users.batchEmpty"))
      return
    }
    if (parsed.users.length > MAX_BATCH_USERS) {
      setError(t("dashboard.pages.users.batchTooMany"))
      return
    }

    setSubmitting(true)
    try {
      const data = await adminPostJson<BatchCreateResult>(
        "/api/admin/user/batch_create",
        { users: parsed.users }
      )
      setResult(data)
      if (data.created + (data.roleFailed ?? 0) > 0) {
        onCreated?.()
      }
      if (data.failed === 0 && (data.roleFailed ?? 0) === 0) {
        setText("")
      }
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t("dashboard.pages.users.batchCreateFailed")
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Button variant="outline" onClick={openDialog}>
        <UserPlus />
        {t("dashboard.pages.users.batchCreate")}
      </Button>

      <DashboardDialog
        open={open}
        onOpenChange={setOpen}
        title={t("dashboard.pages.users.batchTitle")}
        description={t("dashboard.pages.users.batchHint")}
        size="lg"
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
              type="submit"
              form="batch-create-users-form"
              disabled={submitting}
            >
              {submitting
                ? t("dashboard.pages.users.batchCreating")
                : t("dashboard.pages.users.batchSubmit")}
            </Button>
          </>
        }
      >
        <form
          id="batch-create-users-form"
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault()
            void submit()
          }}
        >
          <Textarea
            value={text}
            onChange={(event) => setText(event.target.value)}
            aria-label={t("dashboard.pages.users.batchTitle")}
            placeholder={t("dashboard.pages.users.batchPlaceholder")}
            className="min-h-40 font-mono text-sm"
            disabled={submitting}
          />
          {error ? (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}
          {result ? (
            <div
              className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm"
              aria-live="polite"
            >
              <p>
                {t("dashboard.pages.users.batchResult", {
                  created: result.created,
                  roleFailed: result.roleFailed ?? 0,
                  failed: result.failed,
                })}
              </p>
              {result.errors?.length ? (
                <ul className="max-h-40 space-y-1 overflow-y-auto text-destructive">
                  {result.errors.map((item) => (
                    <li key={`${item.index}-${item.username}`}>
                      #{item.index + 1} {item.username || "-"}: {item.message}
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          ) : null}
        </form>
      </DashboardDialog>
    </>
  )
}
