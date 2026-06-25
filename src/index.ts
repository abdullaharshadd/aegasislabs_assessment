import 'dotenv/config';
import express from 'express';
import { errorHandler } from './middleware/errorHandler';

const app = express();
app.use(express.json());

// TODO: mount routers here
// app.use('/api/users', userRouter);

app.use(errorHandler);

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});

export default app;
