import { useSearchParams } from "react-router"

import { useAppState } from "@/components/app/app-provider"
import { SignupForm } from "@/components/auth/signup-form"
import Link from "@/components/common/link"
import { WidgetCard } from "@/components/common/widget-card"
import { useI18n } from "@/lib/i18n/provider"
import { noindexRouteMeta } from "@/lib/seo"
import { useDocumentTitle } from "@/lib/use-document-title"

export function meta({
  matches,
}: {
  matches: Array<{ data?: unknown; loaderData?: unknown }>
}) {
  return noindexRouteMeta(matches, "Sign up", "注册")
}

export default function SignupRoute() {
  const [searchParams] = useSearchParams()
  const { t } = useI18n()
  const { config } = useAppState()
  const registerEnabled = config?.loginConfig?.registerEnabled !== false
  useDocumentTitle(t("user.signup.title"))
  return (
    <section className="main">
      <div className="container mx-auto">
        <WidgetCard
          className="mx-auto max-w-[600px]"
          title={t("user.signup.title")}
          bodyClassName="pt-0"
        >
          {registerEnabled ? (
            <SignupForm redirect={searchParams.get("redirect") || undefined} />
          ) : (
            <div className="py-12 text-center">
              <p className="text-muted-foreground">
                {t("user.signup.disabled")}
              </p>
              <Link
                href="/user/signin"
                className="mt-4 inline-block text-sm text-primary hover:underline"
              >
                {t("user.signup.backToSignin")}
              </Link>
            </div>
          )}
        </WidgetCard>
      </div>
    </section>
  )
}
