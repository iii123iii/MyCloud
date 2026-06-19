@file:Suppress("UnstableApiUsage")

pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "0.10.0"
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "MyCloud"

// Modules are added here as they are created. Keep this list in sync with the
// directories that actually contain a build.gradle.kts, or Gradle sync fails.
include(":app")
include(":core:common")
include(":core:model")
include(":core:designsystem")
include(":core:network")
include(":core:datastore")
include(":core:database")
include(":core:work")
include(":core:media")
include(":data")
include(":feature:auth")
include(":feature:browser")
include(":feature:sharing")
include(":feature:trash")
include(":feature:photos")
include(":feature:filedetail")
