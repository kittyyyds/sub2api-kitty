import type { ApplicationDefinition, ApplicationFormField } from './types'

export interface CatalogStringOption {
  label: string
  value: string
}

export function catalogField(
  definition: ApplicationDefinition | null | undefined,
  key: string,
): ApplicationFormField | undefined {
  return definition?.form?.fields.find((field) => field.key === key)
}

export function catalogStringOptions(
  definition: ApplicationDefinition | null | undefined,
  key: string,
  fallback: readonly CatalogStringOption[],
): CatalogStringOption[] {
  const options = catalogField(definition, key)?.options
    ?.flatMap((option) => {
      if (typeof option.value !== 'string' || !option.value.trim()) return []
      return [{ label: option.label || option.value, value: option.value }]
    }) || []
  return options.length ? options : [...fallback]
}
