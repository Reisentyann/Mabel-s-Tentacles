<template>
  <div class="register-container">
    <h2>Register</h2>
    <form @submit.prevent="handleRegister">
      <div>
        <label for="username">Username:</label>
        <input type="text" id="username" v-model="username" required />
      </div>
      <div>
        <label for="email">Email (Optional):</label>
        <input type="email" id="email" v-model="email" />
      </div>
      <div>
        <label for="password">Password:</label>
        <input type="password" id="password" v-model="password" required />
      </div>
      <button type="submit">Register</button>
    </form>
    <p v-if="error" class="error">{{ error }}</p>
    <p>Already have an account? <router-link to="/login">Login</router-link></p>
  </div>
</template>

<script setup>
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

const username = ref("");
const email = ref("");
const password = ref("");
const error = ref("");
const router = useRouter();
const authStore = useAuthStore();

const handleRegister = async () => {
  console.log("[DEBUG] handleRegister called with:", {
    username: username.value,
    email: email.value,
  });
  try {
    const data = { username: username.value, password: password.value };
    if (email.value) data.email = email.value;

    console.log("[DEBUG] Calling authStore.registerAction with payload:", data);
    await authStore.registerAction(data);
    console.log(
      "[DEBUG] Registration action completed successfully, attempting auto-login",
    );

    // After register, automatically login
    await authStore.loginAction({
      username: username.value,
      password: password.value,
    });
    console.log("[DEBUG] Auto-login successful, redirecting to files");
    router.push("/files");
  } catch (err) {
    console.error("[DEBUG] Error caught in handleRegister:", err);
    console.error("[DEBUG] err.response:", err.response);
    console.error("[DEBUG] err.message:", err.message);
    error.value =
      err.response?.data?.detail || err.message || "Registration failed";
  }
};
</script>

<style scoped>
.register-container {
  max-width: 400px;
  margin: 0 auto;
  padding: 2rem;
}
.error {
  color: red;
}
form div {
  margin-bottom: 1rem;
}
label {
  display: block;
  margin-bottom: 0.5rem;
}
input {
  width: 100%;
  padding: 0.5rem;
}
</style>
